package replay

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/umputun/tg-spam/lib/spamcheck"
)

type Case struct {
	Msg          string             `json:"msg"`
	UserID       string             `json:"user_id"`
	UserName     string             `json:"user_name"`
	ExpectedSpam bool               `json:"expected_spam"`
	Meta         spamcheck.MetaData `json:"meta"`
}

type Result struct {
	Case       Case                 `json:"case"`
	ActualSpam bool                 `json:"actual_spam"`
	Correct    bool                 `json:"correct"`
	Checks     []spamcheck.Response `json:"checks"`
	Duration   time.Duration        `json:"duration"`
}

type Report struct {
	Total     int      `json:"total"`
	TP        int      `json:"tp"`
	FP        int      `json:"fp"`
	TN        int      `json:"tn"`
	FN        int      `json:"fn"`
	Accuracy  float64  `json:"accuracy"`
	Precision float64  `json:"precision"`
	Recall    float64  `json:"recall"`
	Details   []Result `json:"details"`
}

type CheckFunc func(req spamcheck.Request) (bool, []spamcheck.Response)

func Run(fn CheckFunc, cases []Case) Report {
	report := Report{Details: make([]Result, 0, len(cases))}

	for _, c := range cases {
		req := spamcheck.Request{
			Msg:      c.Msg,
			UserID:   c.UserID,
			UserName: c.UserName,
			Meta:     c.Meta,
		}

		start := time.Now()
		spam, checks := fn(req)
		dur := time.Since(start)

		correct := spam == c.ExpectedSpam
		r := Result{
			Case:       c,
			ActualSpam: spam,
			Correct:    correct,
			Checks:     checks,
			Duration:   dur,
		}

		report.Total++
		if spam && c.ExpectedSpam {
			report.TP++
		} else if spam && !c.ExpectedSpam {
			report.FP++
		} else if !spam && !c.ExpectedSpam {
			report.TN++
		} else {
			report.FN++
		}

		report.Details = append(report.Details, r)
	}

	report.Accuracy = safeDiv(float64(report.TP+report.TN), float64(report.Total))
	report.Precision = safeDiv(float64(report.TP), float64(report.TP+report.FP))
	report.Recall = safeDiv(float64(report.TP), float64(report.TP+report.FN))
	return report
}

func LoadCases(r io.Reader) ([]Case, error) {
	dec := json.NewDecoder(r)
	var cases []Case
	for {
		var c Case
		if err := dec.Decode(&c); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode case: %w", err)
		}
		cases = append(cases, c)
	}
	return cases, nil
}

func LoadCasesFile(path string) ([]Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	return LoadCases(f)
}

func (r *Report) String() string {
	return fmt.Sprintf("total=%d TP=%d FP=%d TN=%d FN=%d accuracy=%.3f precision=%.3f recall=%.3f",
		r.Total, r.TP, r.FP, r.TN, r.FN, r.Accuracy, r.Precision, r.Recall)
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
