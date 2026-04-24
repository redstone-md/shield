package tgspam

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"iter"
	"log"
	"strings"
)

// LoadSamples loads spam samples from a reader and updates the classifier.
// Reset spam, ham samples/classifier, and excluded tokens.
func (d *Detector) LoadSamples(exclReader io.Reader, spamReaders, hamReaders []io.Reader) (LoadResult, error) {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.tokenizedSpam = []map[string]int{}
	d.excludedTokens = map[string]struct{}{}
	d.classifier.reset()

	// excluded tokens should be loaded before spam samples to exclude them from spam tokenization
	for t := range d.readerIterator(exclReader) {
		d.excludedTokens[strings.ToLower(t)] = struct{}{}
	}
	lr := LoadResult{ExcludedTokens: len(d.excludedTokens)}

	// load spam samples and update the classifier with them
	docs := make([]document, 0) //nolint:prealloc // iterator size unknown
	for token := range d.readerIterator(spamReaders...) {
		tokenizedSpam := d.tokenize(token)
		d.tokenizedSpam = append(d.tokenizedSpam, tokenizedSpam) // add to list of samples
		tokens := make([]string, 0, len(tokenizedSpam))
		for token := range tokenizedSpam {
			tokens = append(tokens, token)
		}
		docs = append(docs, newDocument(ClassSpam, tokens...))
		lr.SpamSamples++
	}

	// load ham samples and update the classifier with them
	for token := range d.readerIterator(hamReaders...) {
		tokenizedSpam := d.tokenize(token)
		tokens := make([]string, 0, len(tokenizedSpam))
		for token := range tokenizedSpam {
			tokens = append(tokens, token)
		}
		docs = append(docs, document{spamClass: ClassHam, tokens: tokens})
		lr.HamSamples++
	}

	d.classifier.learn(docs...)
	return lr, nil
}

// LoadStopWords loads stop words from a reader. Reset stop words list before loading.
func (d *Detector) LoadStopWords(readers ...io.Reader) (LoadResult, error) {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.stopWords = []string{}
	for t := range d.readerIterator(readers...) {
		d.stopWords = append(d.stopWords, strings.ToLower(t))
	}
	return LoadResult{StopWords: len(d.stopWords)}, nil
}

// UpdateSpam appends a message to the spam samples file and updates the classifier
func (d *Detector) UpdateSpam(msg string) error {
	return d.updateSample(msg, d.spamSamplesUpd, ClassSpam)
}

// UpdateHam appends a message to the ham samples file and updates the classifier
func (d *Detector) UpdateHam(msg string) error {
	return d.updateSample(msg, d.hamSamplesUpd, ClassHam)
}

// RemoveSpam removes a message from the spam samples file and updates the classifier by unlearning
func (d *Detector) RemoveSpam(msg string) error {
	return d.removeSample(msg, d.spamSamplesUpd, ClassSpam)
}

// RemoveHam removes a message from the ham samples file and updates the classifier by unlearning
func (d *Detector) RemoveHam(msg string) error {
	return d.removeSample(msg, d.hamSamplesUpd, ClassHam)
}

// updateSample appends a message to the samples store and updates the classifier
// doesn't reset state, update append samples
func (d *Detector) updateSample(msg string, upd SampleUpdater, sc spamClass) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	if upd == nil {
		return nil
	}

	// write to dynamic samples storage
	if err := upd.Append(msg); err != nil {
		return fmt.Errorf("can't update %s samples: %w", sc, err)
	}

	// load samples and update the classifier with them
	docs := d.buildDocs(msg, sc)
	d.classifier.learn(docs...)

	// update tokenized spam samples for similarity check
	if sc == ClassSpam {
		tokenizedSpam := d.tokenize(msg)
		d.tokenizedSpam = append(d.tokenizedSpam, tokenizedSpam)
	}

	return nil
}

// removeSample removes a message from the spam samples file and updates the classifier by unlearning
func (d *Detector) removeSample(msg string, upd SampleUpdater, sc spamClass) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	if upd == nil {
		return nil
	}

	// first validate that we can unlearn this sample
	docs := d.buildDocs(msg, sc)
	if err := d.classifier.unlearn(docs...); err != nil {
		return fmt.Errorf("can't unlearn %s samples: %w", sc, err)
	}

	// if unlearn succeeded, remove from storage
	if err := upd.Remove(msg); err != nil {
		// try to relearn since storage update failed
		d.classifier.learn(docs...)
		return fmt.Errorf("can't remove %s samples: %w", sc, err)
	}
	return nil
}

// buildDocs builds a list of classifier documents from a message
func (d *Detector) buildDocs(msg string, sc spamClass) []document {
	docs := make([]document, 0) //nolint:prealloc // iterator size unknown
	for token := range d.readerIterator(bytes.NewBufferString(msg)) {
		tokenizedSample := d.tokenize(token)
		tokens := make([]string, 0, len(tokenizedSample))
		for token := range tokenizedSample {
			tokens = append(tokens, token)
		}
		docs = append(docs, document{spamClass: sc, tokens: tokens})
	}
	return docs
}

// readerIterator parses readers and returns an iterator of data elements, each line is an element.
func (d *Detector) readerIterator(readers ...io.Reader) iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, reader := range readers {
			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				line := scanner.Text()
				// each line with a single element
				cleanToken := strings.Trim(line, " \n\r\t")
				if cleanToken != "" {
					if !yield(cleanToken) {
						return
					}
				}
			}

			if err := scanner.Err(); err != nil {
				log.Printf("[WARN] failed to read tokens, error=%v", err)
			}
		}
	}
}

// tokenize takes a string and returns a map where the keys are unique words (tokens)
// and the values are the frequencies of those words in the string.
// exclude tokens representing common words.
func (d *Detector) tokenize(inp string) map[string]int {
	isExcludedToken := func(token string) bool {
		if _, ok := d.excludedTokens[strings.ToLower(token)]; ok {
			return true
		}
		return false
	}

	tokenFrequency := make(map[string]int)
	for token := range strings.FieldsSeq(inp) {
		if isExcludedToken(token) {
			continue
		}
		token = cleanEmoji(token)
		token = strings.Trim(token, ".,!?-:;()#")
		token = strings.ToLower(token)
		if len([]rune(token)) < 3 {
			continue
		}
		tokenFrequency[strings.ToLower(token)]++
	}
	return tokenFrequency
}
