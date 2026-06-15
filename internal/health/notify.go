package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// NotifyCooldown is the minimum time between desktop notifications for the
	// same signal at the same grade (avoids alert fatigue).
	NotifyCooldown = 30 * time.Minute
)

// Notifier compares successive health states, raises desktop notifications on
// state change, and persists status.json. It is fully offline; ntfy is
// optional and only used when NtfyURL is set.
type Notifier struct {
	reader  *StateReader
	NtfyURL string // optional LAN-only ntfy endpoint, e.g. "http://vault:8080/redoubt"
}

// NewNotifier creates a Notifier backed by the given reader.
func NewNotifier(reader *StateReader) *Notifier {
	return &Notifier{reader: reader}
}

// Notify evaluates the current health state, writes status.json, and fires
// notifications for any signal that changed grade since the last call
// (with cooldown/dedupe).
func (n *Notifier) Notify(ctx context.Context, current Status) error {
	if err := n.reader.WriteStatus(current); err != nil {
		return fmt.Errorf("writing status.json: %w", err)
	}

	alertState, _ := n.reader.ReadAlertState()
	if alertState.LastGrades == nil {
		alertState.LastGrades = make(map[string]Grade)
	}

	now := current.EvaluatedAt
	changed := false

	for signal, ss := range current.Signals {
		prev := alertState.LastGrades[signal]
		if ss.Grade == prev {
			continue
		}

		cooling := now.Sub(alertState.LastNotifiedAt) < NotifyCooldown
		recovery := ss.Grade < prev
		if cooling && !recovery {
			continue
		}

		if err := n.send(ctx, signal, ss); err != nil {
			fmt.Printf("notify: %v\n", err)
		}
		alertState.LastGrades[signal] = ss.Grade
		changed = true
	}

	if changed {
		alertState.LastNotifiedAt = now
		_ = n.reader.WriteAlertState(alertState)
	}

	return nil
}

func (n *Notifier) send(ctx context.Context, signal string, ss SignalStatus) error {
	title, body := buildMessage(signal, ss)

	if err := platformNotify("Redoubt — "+title, body); err != nil {
		fmt.Printf("desktop notify: %v\n", err)
	}

	if n.NtfyURL != "" {
		if err := sendNtfy(ctx, n.NtfyURL, title, body, ss.Grade); err != nil {
			return fmt.Errorf("ntfy: %w", err)
		}
	}
	return nil
}

func buildMessage(signal string, ss SignalStatus) (title, body string) {
	label := strings.ReplaceAll(signal, "_", " ")
	switch ss.Grade {
	case GradeCritical:
		title = fmt.Sprintf("%s — CRITICAL", label)
	case GradeWarning:
		title = fmt.Sprintf("%s — warning", label)
	case GradeHealthy:
		title = fmt.Sprintf("%s — recovered", label)
	default:
		title = fmt.Sprintf("%s — unknown", label)
	}
	body = ss.Message
	return title, body
}

func sendNtfy(ctx context.Context, url, title, body string, grade Grade) error {
	priority := "default"
	switch grade {
	case GradeCritical:
		priority = "urgent"
	case GradeWarning:
		priority = "high"
	case GradeHealthy:
		priority = "low"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	req.Header.Set("Priority", priority)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ntfy returned %d", resp.StatusCode)
	}
	return nil
}

// CheckAndNotify is the entry point for a scheduled health-check run.
// It evaluates health, writes status.json, and sends any needed notifications.
func CheckAndNotify(ctx context.Context, dir string, thresholds Thresholds, ntfyURL string) (Status, error) {
	reader := NewStateReader(dir)
	status, err := Evaluate(reader, thresholds, time.Now().UTC())
	if err != nil {
		return Status{}, err
	}

	n := NewNotifier(reader)
	n.NtfyURL = ntfyURL
	if err := n.Notify(ctx, status); err != nil {
		return status, err
	}
	return status, nil
}

// jsonStatus is the machine-readable representation for --json output.
type jsonStatus struct {
	Overall     string                `json:"overall"`
	Signals     map[string]jsonSignal `json:"signals"`
	EvaluatedAt time.Time             `json:"evaluated_at"`
}

type jsonSignal struct {
	Grade   string `json:"grade"`
	Message string `json:"message"`
}

// MarshalJSON produces the canonical status.json format.
func (s Status) MarshalJSON() ([]byte, error) {
	js := jsonStatus{
		Overall:     s.Overall.String(),
		EvaluatedAt: s.EvaluatedAt,
		Signals:     make(map[string]jsonSignal, len(s.Signals)),
	}
	for k, v := range s.Signals {
		js.Signals[k] = jsonSignal{Grade: v.Grade.String(), Message: v.Message}
	}
	return json.Marshal(js)
}

// UnmarshalJSON decodes the canonical status.json format.
func (s *Status) UnmarshalJSON(data []byte) error {
	var js jsonStatus
	if err := json.Unmarshal(data, &js); err != nil {
		return err
	}
	_ = s.Overall.UnmarshalText([]byte(js.Overall))
	s.EvaluatedAt = js.EvaluatedAt
	s.Signals = make(map[string]SignalStatus, len(js.Signals))
	for k, v := range js.Signals {
		var g Grade
		_ = g.UnmarshalText([]byte(v.Grade))
		s.Signals[k] = SignalStatus{Grade: g, Message: v.Message}
	}
	return nil
}
