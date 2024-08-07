package email

type EmailData struct {
	to   string
	subj string
	body string
}

func NewEmailData(to, subj, body string) *EmailData {
	return &EmailData{
		to:   to,
		subj: subj,
		body: body,
	}
}

func (e *EmailData) To() string {
	return e.to
}

func (e *EmailData) Subject() string {
	return e.subj
}

func (e *EmailData) Body() string {
	return e.body
}
