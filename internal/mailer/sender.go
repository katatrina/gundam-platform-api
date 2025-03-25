package mailer

const (
	senderEmailName    = "Mecha World"
	senderEmailAddress = "mechaworldcapstone@gmail.com"
)

type EmailHeader struct {
	Subject string
	To      []string
}
