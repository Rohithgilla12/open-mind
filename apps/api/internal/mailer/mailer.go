// Package mailer sends multipart e-mail messages, with optional binary
// attachments, over SMTP using only the standard library.
package mailer

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// SMTPConfig holds the connection and authentication details for an SMTP
// server. Auth is skipped when Username is empty.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// Attachment is a single binary attachment carried on a Message.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// Message is a single e-mail to send. Attachment is optional.
type Message struct {
	To         string
	Subject    string
	BodyText   string
	Attachment *Attachment
}

// Mailer sends e-mail messages.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

const dialTimeout = 15 * time.Second

const base64LineLength = 76

type smtpMailer struct {
	cfg SMTPConfig
}

// New returns a Mailer that sends messages over SMTP using cfg. Port 465
// uses implicit TLS; any other port dials in the clear and upgrades via
// STARTTLS only if the server advertises support for it.
func New(cfg SMTPConfig) Mailer {
	return &smtpMailer{cfg: cfg}
}

func (m *smtpMailer) Send(ctx context.Context, msg Message) error {
	conn, err := m.dial(ctx)
	if err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("creating smtp client: %w", err)
	}
	defer client.Close()

	if m.cfg.Port != 465 {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: m.cfg.Host}); err != nil {
				return fmt.Errorf("starttls: %w", err)
			}
		}
	}

	if m.cfg.Username != "" {
		auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := client.Mail(m.cfg.From); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := client.Rcpt(msg.To); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}

	body, err := buildMessage(m.cfg, msg)
	if err != nil {
		_ = w.Close()
		return fmt.Errorf("building message: %w", err)
	}

	if _, err := w.Write(body); err != nil {
		_ = w.Close()
		return fmt.Errorf("writing message body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing data writer: %w", err)
	}

	// The message was already accepted by the server (Data/Close above
	// succeeded), so the mail is delivered regardless of how the session
	// teardown goes. A failed QUIT doesn't undo that, so it's not a Send
	// failure — just let the deferred client.Close() clean up the connection.
	_ = client.Quit()
	return nil
}

func (m *smtpMailer) dial(ctx context.Context) (net.Conn, error) {
	addr := net.JoinHostPort(m.cfg.Host, strconv.Itoa(m.cfg.Port))
	dialer := &net.Dialer{Timeout: dialTimeout}

	if m.cfg.Port == 465 {
		rawConn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("dialing smtp server: %w", err)
		}
		tlsConn := tls.Client(rawConn, &tls.Config{ServerName: m.cfg.Host})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = rawConn.Close()
			return nil, fmt.Errorf("tls handshake: %w", err)
		}
		return tlsConn, nil
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dialing smtp server: %w", err)
	}
	return conn, nil
}

// buildMessage renders msg into a full RFC 5322 message with CRLF line
// endings: headers, a text/plain body, and (if present) a base64-encoded
// attachment part, joined as multipart/mixed.
func buildMessage(cfg SMTPConfig, msg Message) ([]byte, error) {
	var bodyBuf bytes.Buffer
	mpw := multipart.NewWriter(&bodyBuf)

	textHeader := textproto.MIMEHeader{}
	textHeader.Set("Content-Type", `text/plain; charset="utf-8"`)
	// BodyText may carry arbitrary UTF-8 (em-dashes, curly quotes, etc.), so
	// it can't be declared 7bit. We assume 8BITMIME support here (we don't
	// negotiate the extension explicitly, but every server we target in
	// practice for this self-host mailer advertises it) rather than
	// quoted-printable/base64-encoding the body.
	textHeader.Set("Content-Transfer-Encoding", "8bit")
	textPart, err := mpw.CreatePart(textHeader)
	if err != nil {
		return nil, fmt.Errorf("creating text part: %w", err)
	}
	if _, err := textPart.Write([]byte(toCRLF(msg.BodyText))); err != nil {
		return nil, fmt.Errorf("writing text part: %w", err)
	}

	if msg.Attachment != nil {
		if err := writeAttachmentPart(mpw, msg.Attachment); err != nil {
			return nil, err
		}
	}

	if err := mpw.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart writer: %w", err)
	}

	var out bytes.Buffer
	fmt.Fprintf(&out, "From: %s\r\n", headerValue(cfg.From))
	fmt.Fprintf(&out, "To: %s\r\n", headerValue(msg.To))
	fmt.Fprintf(&out, "Subject: %s\r\n", headerValue(msg.Subject))
	fmt.Fprintf(&out, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&out, "Content-Type: multipart/mixed; boundary=%q\r\n", mpw.Boundary())
	out.WriteString("\r\n")
	out.Write(bodyBuf.Bytes())

	return out.Bytes(), nil
}

// headerValue strips CR/LF so caller-supplied text (e.g. a Subject derived
// from a user-titled item or lens) cannot inject additional MIME headers.
func headerValue(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.ReplaceAll(s, "\n", " ")
}

func writeAttachmentPart(mpw *multipart.Writer, att *Attachment) error {
	contentType := att.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	header := textproto.MIMEHeader{}
	header.Set("Content-Type", contentType)
	header.Set("Content-Transfer-Encoding", "base64")
	header.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, att.Filename))

	part, err := mpw.CreatePart(header)
	if err != nil {
		return fmt.Errorf("creating attachment part: %w", err)
	}
	if _, err := part.Write([]byte(wrapBase64(att.Data))); err != nil {
		return fmt.Errorf("writing attachment part: %w", err)
	}
	return nil
}

func toCRLF(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}

func wrapBase64(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	var b strings.Builder
	for i := 0; i < len(encoded); i += base64LineLength {
		end := i + base64LineLength
		if end > len(encoded) {
			end = len(encoded)
		}
		b.WriteString(encoded[i:end])
		b.WriteString("\r\n")
	}
	return b.String()
}
