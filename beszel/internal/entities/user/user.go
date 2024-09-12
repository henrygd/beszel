package user

type UserSettings struct {
	// Language             string   `json:"lang"`
	ChartTime            string   `json:"chartTime"`
	NotificationEmails   []string `json:"emails"`
	NotificationWebhooks []string `json:"webhooks"`
}
