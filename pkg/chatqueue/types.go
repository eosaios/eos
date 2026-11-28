package chatqueue

type Message[A any] struct {
	ID          string `json:"id"`
	Content     string `json:"content"`
	Attachments []A    `json:"attachments"`
	Reasoning   string `json:"reasoning,omitempty"`
}

type State[A any] struct {
	Messages []Message[A] `json:"messages"`
	Paused   bool         `json:"paused"`
	Starting string       `json:"starting,omitempty"`
}

type Input struct {
	ID          string   `json:"id"`
	Content     string   `json:"content"`
	Attachments []string `json:"attachments"`
	Reasoning   string   `json:"reasoning"`
	Mode        string   `json:"mode"`
}
