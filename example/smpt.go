package main

import (
	"fmt"
	"net/smtp"
)

func main() {
	from := "sender@infrasutra.com"
	to := "receiver@infrasutra.com"

	for i := 1; i <= 1000; i++ {
		subject := fmt.Sprintf("LocalSMTP Example #%d", i)
		body := fmt.Sprintf("Hello from LocalSMTP. Message %d.\r\n", i)
		message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", from, to, subject, body)

		if err := smtp.SendMail("127.0.0.1:2025", nil, from, []string{to}, []byte(message)); err != nil {
			panic(err)
		}
	}

	fmt.Println("sent 1000 messages")
}
