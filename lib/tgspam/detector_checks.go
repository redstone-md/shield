package tgspam

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/forPelevin/gomoji"
	"github.com/go-pkgz/repeater"
	"github.com/go-pkgz/repeater/strategy"

	"github.com/umputun/tg-spam/lib/spamcheck"
	"github.com/umputun/tg-spam/lib/textnorm"
)

// isSpam checks if a given message is similar to any of the known bad messages
func (d *Detector) isSpamSimilarityHigh(msg string) spamcheck.Response {
	// check for spam similarity
	tokenizedMessage := d.tokenize(msg)
	maxSimilarity := 0.0
	for _, spam := range d.tokenizedSpam {
		similarity := d.cosineSimilarity(tokenizedMessage, spam)
		if similarity > maxSimilarity {
			maxSimilarity = similarity
		}
		if similarity >= d.SimilarityThreshold {
			return spamcheck.Response{
				Spam: true, Name: "similarity",
				Details:        fmt.Sprintf("%0.2f/%0.2f", maxSimilarity, d.SimilarityThreshold),
				RuleID:         "similarity",
				Score:          maxSimilarity,
				Weight:         1.0,
				NormalizedText: truncateRunes(msg),
			}
		}
	}
	return spamcheck.Response{Spam: false, Name: "similarity",
		Details: fmt.Sprintf("%0.2f/%0.2f", maxSimilarity, d.SimilarityThreshold)}
}

// cosineSimilarity calculates the cosine similarity between two token frequency maps.
func (d *Detector) cosineSimilarity(a, b map[string]int) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}

	dotProduct := 0      // sum of product of corresponding frequencies
	normA, normB := 0, 0 // square root of sum of squares of frequencies

	for key, val := range a {
		dotProduct += val * b[key]
		normA += val * val
	}
	for _, val := range b {
		normB += val * val
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	// cosine similarity formula
	return float64(dotProduct) / (math.Sqrt(float64(normA)) * math.Sqrt(float64(normB)))
}

// isCasSpam checks if a given user ID is a spammer with CAS API.
func (d *Detector) isCasSpam(msgID string) spamcheck.Response {
	if msgID == "" {
		return spamcheck.Response{Spam: false, Name: "cas", Details: "check disabled"}
	}
	if _, err := strconv.ParseInt(msgID, 10, 64); err != nil {
		return spamcheck.Response{Spam: false, Name: "cas", Details: fmt.Sprintf("invalid user id %q", msgID)}
	}
	reqURL := fmt.Sprintf("%s/check?user_id=%s", d.CasAPI, msgID)
	req, err := http.NewRequest("GET", reqURL, http.NoBody)
	if err != nil {
		return spamcheck.Response{Spam: false, Name: "cas", Details: fmt.Sprintf("failed to make request %s: %v", reqURL, err)}
	}

	if d.CasUserAgent != "" {
		req.Header.Set("User-Agent", d.CasUserAgent)
	}

	var resp *http.Response
	// wrap HTTP call with retry logic: 3 attempts, 500ms initial delay, exponential backoff with jitter
	rptr := repeater.New(&strategy.Backoff{
		Repeats:  3,
		Duration: 500 * time.Millisecond,
		Factor:   2.0,
		Jitter:   true,
	})

	err = rptr.Do(context.Background(), func() error {
		var httpErr error
		resp, httpErr = d.HTTPClient.Do(req)
		if httpErr != nil {
			return fmt.Errorf("http request failed: %w", httpErr) // retry on network errors
		}

		// retry on 5xx server errors
		if resp.StatusCode >= 500 {
			_ = resp.Body.Close() // ignore close error on retry
			return fmt.Errorf("server error: %d", resp.StatusCode)
		}

		// retry on non-200 status
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return fmt.Errorf("unexpected status: %d", resp.StatusCode)
		}

		// retry on HTML responses (issue #325)
		contentType := resp.Header.Get("Content-Type")
		if contentType != "" && !strings.Contains(contentType, "application/json") {
			_ = resp.Body.Close()
			return fmt.Errorf("unexpected content type: %s", contentType)
		}

		return nil // success - exit retry loop
	})

	if err != nil {
		log.Printf("[WARN] CAS API request failed for user %s after retries: %v", msgID, err)
		return spamcheck.Response{Spam: false, Name: "cas", Details: fmt.Sprintf("failed to send request %s: %v", reqURL, err)}
	}
	defer resp.Body.Close()

	respData := struct {
		OK          bool   `json:"ok"` // ok means user is a spammer
		Description string `json:"description"`
	}{}

	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		log.Printf("[WARN] CAS API response parse error for user %s: %v", msgID, err)
		return spamcheck.Response{Spam: false, Name: "cas", Details: fmt.Sprintf("failed to parse response from %s: %v", reqURL, err)}
	}
	respData.Description = strings.ToLower(respData.Description)
	respData.Description = strings.TrimSuffix(respData.Description, ".")

	if respData.OK {
		if respData.Description == "" {
			respData.Description = "spam detected"
		}
		return spamcheck.Response{Name: "cas", Spam: true, Details: respData.Description,
			RuleID: "cas", Score: 1.0, Weight: 1.0, NormalizedText: msgID}
	}
	details := respData.Description
	if details == "" {
		details = "not found"
	}
	return spamcheck.Response{Name: "cas", Spam: false, Details: details}
}

// isSpamClassified classify tokens from a document
func (d *Detector) isSpamClassified(msg string) spamcheck.Response {
	tm := d.tokenize(msg)
	tokens := make([]string, 0, len(tm))
	for token := range tm {
		tokens = append(tokens, token)
	}
	class, prob, certain := d.classifier.classify(tokens...)
	isSpam := class == ClassSpam && certain && (d.MinSpamProbability == 0 || prob >= d.MinSpamProbability)

	// handle NaN or infinite probability values
	probStr := "0.00"
	if !math.IsNaN(prob) && !math.IsInf(prob, 0) {
		probStr = fmt.Sprintf("%.2f", prob)
	}

	return spamcheck.Response{Name: "classifier", Spam: isSpam,
		Details: fmt.Sprintf("probability of %s: %s%%", class, probStr),
		RuleID:  "classifier", Score: prob, Weight: 1.0, NormalizedText: truncateRunes(msg)}
}

// isStopWord checks if a given message or username contains any of the stop words.
// stop words prefixed with "=" require exact match (whole text equals the word),
// otherwise substring match is used.
func (d *Detector) isStopWord(msg string, req spamcheck.Request) spamcheck.Response {
	// check message text
	cleanMsg := normalizeLookupText(cleanEmoji(msg))
	for _, word := range d.stopWords {
		if matchStopWord(cleanMsg, word) {
			return withScoring(spamcheck.Response{Name: "stopword", Spam: true, Details: strings.TrimPrefix(word, "=")}, cleanMsg)
		}
	}

	names := []string{}
	if req.UserName != "" {
		names = append(names, req.UserName)
	}
	if req.UserID != "" {
		names = append(names, req.UserID)
	}
	for _, name := range names {
		normalizedName := normalizeLookupText(name)
		for _, word := range d.stopWords {
			if matchStopWord(normalizedName, word) {
				return withScoring(spamcheck.Response{Name: "stopword", Spam: true, Details: strings.TrimPrefix(word, "=")}, normalizedName)
			}
		}
	}

	return spamcheck.Response{Name: "stopword", Spam: false, Details: "not found"}
}

// matchStopWord checks if text matches a stop word.
// if word starts with "=", exact match is required (text must equal word).
// otherwise, substring match is used (text must contain word).
func matchStopWord(text, word string) bool {
	if checkWord, found := strings.CutPrefix(word, "="); found {
		// exact match: text must equal the word (without prefix)
		if checkWord == "" {
			return false // skip invalid "=" only pattern
		}
		normalizedWord := normalizeLookupText(checkWord) // word already lowercased at load time
		return text == normalizedWord
	}
	// substring match
	normalizedWord := normalizeLookupText(word) // word already lowercased at load time
	return strings.Contains(text, normalizedWord)
}

// isManyEmojis checks if a given message contains more than MaxAllowedEmoji emojis.
func (d *Detector) isManyEmojis(msg string) spamcheck.Response {
	count := countEmoji(msg)
	return withScoring(spamcheck.Response{Name: "emoji", Spam: count > d.MaxAllowedEmoji,
		Details: fmt.Sprintf("%d/%d", count, d.MaxAllowedEmoji)}, msg)
}

// isMultiLang checks if a given message contains more than MultiLangWords multi-lingual words.
func (d *Detector) isMultiLang(msg string) spamcheck.Response {
	isMultiLingual := func(word string) bool {
		scripts := make(map[string]bool)
		for _, r := range word {
			if r == 'i' || unicode.IsSpace(r) || unicode.IsNumber(r) { // skip 'i' (common in many langs) and spaces
				continue
			}

			scriptFound := false
			for name, table := range unicode.Scripts {
				if unicode.Is(table, r) {
					if name != "Common" && name != "Inherited" {
						scripts[name] = true
						if len(scripts) > 1 {
							return true
						}
						scriptFound = true
					}
					break
				}
			}

			// if no specific script was found, it might be a symbol or punctuation
			if !scriptFound {
				// check for mathematical alphanumeric symbols and letterlike symbols
				if unicode.In(r, unicode.Other_Math, unicode.Other_Alphabetic) ||
					(r >= '\U0001D400' && r <= '\U0001D7FF') || // mathematical Alphanumeric Symbols
					(r >= '\u2100' && r <= '\u214F') { // letterlike Symbols
					scripts["Mathematical"] = true
					if len(scripts) > 1 {
						return true
					}
				} else if !unicode.IsPunct(r) && !unicode.IsSymbol(r) {
					// if it's not punctuation or a symbol, count it as "Other"
					scripts["Other"] = true
					if len(scripts) > 1 {
						return true
					}
				}
			}
		}
		return false
	}

	count := 0
	words := strings.FieldsFunc(msg, func(r rune) bool {
		return unicode.IsSpace(r) || r == '-'
	})
	for _, word := range words {
		if isMultiLingual(word) {
			count++
		}
	}
	if count >= d.MultiLangWords {
		return withScoring(spamcheck.Response{
			Name: "multi-lingual", Spam: true,
			Details: fmt.Sprintf("%d/%d", count, d.MultiLangWords),
		}, msg)
	}
	return spamcheck.Response{Name: "multi-lingual", Spam: false, Details: fmt.Sprintf("%d/%d", count, d.MultiLangWords)}
}

// isAbnormalSpacing detects abnormal spacing patterns used to evade filters
// things like this: "w o r d s p a c i n g some thing he re blah blah"
func (d *Detector) isAbnormalSpacing(msg string) spamcheck.Response {
	text := strings.ToUpper(msg)

	// quick check for empty or very short text
	if len(text) < 10 {
		return spamcheck.Response{
			Name:    "word-spacing",
			Spam:    false,
			Details: "too short",
		}
	}

	words := strings.Fields(text)
	// check for minimum number of words
	if len(words) < d.AbnormalSpacing.MinWordsCount {
		return spamcheck.Response{
			Name:    "word-spacing",
			Spam:    false,
			Details: fmt.Sprintf("too few words (%d)", len(words)),
		}
	}

	// count letters and spaces in original text
	var totalChars, spaces int
	for _, r := range text {
		if unicode.IsLetter(r) {
			totalChars++
		} else if unicode.IsSpace(r) {
			spaces++
		}
	}

	// look for suspicious word lengths and spacing patterns
	shortWords := 0
	if d.AbnormalSpacing.ShortWordLen > 0 { // if ShortWordLen is 0, skip short word detection
		for _, word := range words {
			wordRunes := []rune(word)
			if len(wordRunes) <= d.AbnormalSpacing.ShortWordLen && len(wordRunes) > 0 {
				shortWords++
			}
		}
	}

	// safety check
	if spaces == 0 || totalChars == 0 {
		return spamcheck.Response{
			Name:    "word-spacing",
			Spam:    false,
			Details: "no spaces or letters",
		}
	}

	// calculate ratios
	spaceRatio := float64(spaces) / float64(totalChars)
	shortWordRatio := float64(shortWords) / float64(len(words))
	if shortWordRatio > d.AbnormalSpacing.ShortWordRatioThreshold || spaceRatio > d.AbnormalSpacing.SpaceRatioThreshold {
		return withScoring(spamcheck.Response{
			Name:    "word-spacing",
			Spam:    true,
			Details: fmt.Sprintf("abnormal (ratio: %.2f, short: %.0f%%)", spaceRatio, shortWordRatio*100),
		}, msg)
	}

	return spamcheck.Response{
		Name:    "word-spacing",
		Spam:    false,
		Details: fmt.Sprintf("normal (ratio: %.2f, short: %.0f%%)", spaceRatio, shortWordRatio*100),
	}
}

// cleanText removes control and format characters from a given text
func (d *Detector) cleanText(text string) string {
	return textnorm.New(textnorm.Options{StripInvisible: true}).Normalize(text)
}

func cleanEmoji(s string) string {
	return gomoji.RemoveEmojis(s)
}

func countEmoji(s string) int {
	return len(gomoji.CollectAll(s))
}

func normalizeLookupText(text string) string {
	return textnorm.New(textnorm.Options{
		LowerCase:           true,
		Trim:                true,
		CanonicalWhitespace: true,
	}).Normalize(text)
}
