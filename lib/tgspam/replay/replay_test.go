package replay

import (
	"bytes"
	"strings"
	"testing"

	"github.com/umputun/tg-spam/lib/spamcheck"
)

func TestRun_AllCorrect(t *testing.T) {
	cases := []Case{
		{Msg: "spam message", UserID: "1", ExpectedSpam: true},
		{Msg: "hello world", UserID: "2", ExpectedSpam: false},
		{Msg: "buy viagra", UserID: "3", ExpectedSpam: true},
		{Msg: "nice day", UserID: "4", ExpectedSpam: false},
	}

	fn := func(req spamcheck.Request) (bool, []spamcheck.Response) {
		spam := strings.Contains(req.Msg, "spam") || strings.Contains(req.Msg, "viagra")
		return spam, []spamcheck.Response{{Name: "keyword", Spam: spam}}
	}

	report := Run(fn, cases)

	if report.Total != 4 {
		t.Fatalf("total=%d want 4", report.Total)
	}
	if report.TP != 2 {
		t.Errorf("TP=%d want 2", report.TP)
	}
	if report.TN != 2 {
		t.Errorf("TN=%d want 2", report.TN)
	}
	if report.FP != 0 {
		t.Errorf("FP=%d want 0", report.FP)
	}
	if report.FN != 0 {
		t.Errorf("FN=%d want 0", report.FN)
	}
	if report.Accuracy != 1.0 {
		t.Errorf("accuracy=%.3f want 1.000", report.Accuracy)
	}
}

func TestRun_WithErrors(t *testing.T) {
	cases := []Case{
		{Msg: "spam", UserID: "1", ExpectedSpam: true},
		{Msg: "clean", UserID: "2", ExpectedSpam: false},
		{Msg: "spam", UserID: "3", ExpectedSpam: false},
		{Msg: "clean", UserID: "4", ExpectedSpam: true},
	}

	fn := func(req spamcheck.Request) (bool, []spamcheck.Response) {
		spam := strings.Contains(req.Msg, "spam")
		return spam, nil
	}

	report := Run(fn, cases)

	if report.TP != 1 {
		t.Errorf("TP=%d want 1", report.TP)
	}
	if report.FP != 1 {
		t.Errorf("FP=%d want 1", report.FP)
	}
	if report.TN != 1 {
		t.Errorf("TN=%d want 1", report.TN)
	}
	if report.FN != 1 {
		t.Errorf("FN=%d want 1", report.FN)
	}
	if report.Accuracy != 0.5 {
		t.Errorf("accuracy=%.3f want 0.500", report.Accuracy)
	}
	if report.Precision != 0.5 {
		t.Errorf("precision=%.3f want 0.500", report.Precision)
	}
	if report.Recall != 0.5 {
		t.Errorf("recall=%.3f want 0.500", report.Recall)
	}
}

func TestRun_Empty(t *testing.T) {
	fn := func(req spamcheck.Request) (bool, []spamcheck.Response) {
		return false, nil
	}
	report := Run(fn, nil)

	if report.Total != 0 {
		t.Errorf("total=%d want 0", report.Total)
	}
	if report.Accuracy != 0 {
		t.Errorf("accuracy=%.3f want 0", report.Accuracy)
	}
}

func TestLoadCases(t *testing.T) {
	input := `{"msg":"spam","user_id":"1","expected_spam":true}
{"msg":"clean","user_id":"2","expected_spam":false}
`
	cases, err := LoadCases(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 {
		t.Fatalf("cases=%d want 2", len(cases))
	}
	if cases[0].Msg != "spam" || !cases[0].ExpectedSpam {
		t.Errorf("case0=%+v", cases[0])
	}
	if cases[1].Msg != "clean" || cases[1].ExpectedSpam {
		t.Errorf("case1=%+v", cases[1])
	}
}

func TestReport_String(t *testing.T) {
	r := &Report{Total: 10, TP: 5, FP: 1, TN: 3, FN: 1, Accuracy: 0.8, Precision: 0.833, Recall: 0.833}
	s := r.String()
	if !strings.Contains(s, "total=10") || !strings.Contains(s, "TP=5") {
		t.Errorf("string=%s", s)
	}
}

func TestRun_DetailCorrectness(t *testing.T) {
	cases := []Case{
		{Msg: "bad", UserID: "1", ExpectedSpam: true},
		{Msg: "good", UserID: "2", ExpectedSpam: false},
	}

	fn := func(req spamcheck.Request) (bool, []spamcheck.Response) {
		spam := req.Msg == "bad"
		return spam, []spamcheck.Response{{Name: "test", Spam: spam, Details: req.Msg}}
	}

	report := Run(fn, cases)

	if len(report.Details) != 2 {
		t.Fatalf("details=%d want 2", len(report.Details))
	}
	if !report.Details[0].Correct {
		t.Errorf("detail0 should be correct")
	}
	if !report.Details[1].Correct {
		t.Errorf("detail1 should be correct")
	}
	if report.Details[0].Checks[0].Name != "test" {
		t.Errorf("detail0 check name=%s", report.Details[0].Checks[0].Name)
	}
	if report.Details[0].Duration == 0 {
		t.Errorf("detail0 duration should be >0")
	}
}
