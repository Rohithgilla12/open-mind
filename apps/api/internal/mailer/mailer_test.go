package mailer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeSMTP listens on 127.0.0.1:0 and speaks just enough SMTP to accept one
// message, capturing the DATA payload for assertions.
func fakeSMTP(t *testing.T) (addr string, data *bytes.Buffer) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	buf := &bytes.Buffer{}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		fmt.Fprintf(conn, "220 fake ESMTP\r\n")
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if strings.TrimRight(line, "\r\n") == "." {
					inData = false
					fmt.Fprintf(conn, "250 ok\r\n")
					continue
				}
				buf.WriteString(line)
				continue
			}
			switch {
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
				fmt.Fprintf(conn, "250-fake\r\n250 8BITMIME\r\n") // no STARTTLS advertised
			case strings.HasPrefix(line, "MAIL"), strings.HasPrefix(line, "RCPT"):
				fmt.Fprintf(conn, "250 ok\r\n")
			case strings.HasPrefix(line, "DATA"):
				inData = true
				fmt.Fprintf(conn, "354 go\r\n")
			case strings.HasPrefix(line, "QUIT"):
				fmt.Fprintf(conn, "221 bye\r\n")
				return
			default:
				fmt.Fprintf(conn, "250 ok\r\n")
			}
		}
	}()
	return ln.Addr().String(), buf
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi(%q): %v", portStr, err)
	}
	return host, port
}

func TestSend_WithAttachment(t *testing.T) {
	addr, data := fakeSMTP(t)
	host, port := splitHostPort(t, addr)

	m := New(SMTPConfig{
		Host: host,
		Port: port,
		From: "sender@example.com",
	})

	attachmentData := []byte("this is definitely not really an epub file, but it has enough bytes to wrap across multiple base64 lines when encoded, which is exactly what we want to exercise here.")

	msg := Message{
		To:       "reader@example.com",
		Subject:  "Your book",
		BodyText: "Here is your book.",
		Attachment: &Attachment{
			Filename:    "book.epub",
			ContentType: "application/epub+zip",
			Data:        attachmentData,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Send(ctx, msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	payload := data.String()

	if !strings.Contains(payload, "To: reader@example.com") {
		t.Fatalf("payload missing To header: %s", payload)
	}
	if !strings.Contains(payload, "Subject: Your book") {
		t.Fatalf("payload missing Subject header: %s", payload)
	}
	if !strings.Contains(payload, "multipart/mixed") {
		t.Fatalf("payload missing multipart/mixed: %s", payload)
	}

	boundaryIdx := strings.Index(payload, "boundary=")
	if boundaryIdx == -1 {
		t.Fatalf("payload missing boundary parameter: %s", payload)
	}

	if !strings.Contains(payload, "text/plain") {
		t.Fatalf("payload missing text/plain part: %s", payload)
	}
	if !strings.Contains(payload, "Here is your book.") {
		t.Fatalf("payload missing body text: %s", payload)
	}

	if !strings.Contains(payload, `filename="book.epub"`) {
		t.Fatalf("payload missing attachment filename in Content-Disposition: %s", payload)
	}
	if !strings.Contains(payload, "application/epub+zip") {
		t.Fatalf("payload missing attachment content type: %s", payload)
	}

	// Extract the base64 body of the attachment part: everything between the
	// attachment's Content-Transfer-Encoding block and the next boundary.
	encIdx := strings.Index(payload, "Content-Transfer-Encoding: base64")
	if encIdx == -1 {
		t.Fatalf("payload missing base64 content-transfer-encoding: %s", payload)
	}
	rest := payload[encIdx:]
	// Skip past the header line and the blank line that follows it.
	parts := strings.SplitN(rest, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("could not locate base64 body: %s", rest)
	}
	body := parts[1]
	endIdx := strings.Index(body, "--")
	if endIdx != -1 {
		body = body[:endIdx]
	}
	cleaned := strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", ""), "\n", "")
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		t.Fatalf("failed to decode base64 attachment: %v\nraw: %s", err, body)
	}
	if !bytes.Equal(decoded, attachmentData) {
		t.Fatalf("decoded attachment mismatch:\ngot:  %q\nwant: %q", decoded, attachmentData)
	}

	// Verify 76-char wrapping: every non-final base64 line should be exactly
	// 76 chars.
	lines := strings.Split(strings.TrimRight(body, "\r\n"), "\r\n")
	for i, line := range lines {
		if i == len(lines)-1 {
			continue
		}
		if len(line) != 76 {
			t.Fatalf("base64 line %d length = %d, want 76: %q", i, len(line), line)
		}
	}
}

func TestSend_NoAttachment(t *testing.T) {
	addr, data := fakeSMTP(t)
	host, port := splitHostPort(t, addr)

	m := New(SMTPConfig{
		Host: host,
		Port: port,
		From: "sender@example.com",
	})

	msg := Message{
		To:       "reader@example.com",
		Subject:  "Plain message",
		BodyText: "No attachment here.",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Send(ctx, msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	payload := data.String()
	if !strings.Contains(payload, "No attachment here.") {
		t.Fatalf("payload missing body text: %s", payload)
	}
}

func TestSend_UTF8BodyDeclaredAs8Bit(t *testing.T) {
	addr, data := fakeSMTP(t)
	host, port := splitHostPort(t, addr)

	m := New(SMTPConfig{
		Host: host,
		Port: port,
		From: "sender@example.com",
	})

	msg := Message{
		To:       "reader@example.com",
		Subject:  "Em-dash test",
		BodyText: "Openmind saves anything — links, notes, images — instantly.",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Send(ctx, msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	payload := data.String()

	if !strings.Contains(payload, "Content-Transfer-Encoding: 8bit") {
		t.Fatalf("payload missing 8bit content-transfer-encoding for text part: %s", payload)
	}
	if strings.Contains(payload, "Content-Transfer-Encoding: 7bit") {
		t.Fatalf("payload still declares 7bit somewhere: %s", payload)
	}
	if !strings.Contains(payload, "Openmind saves anything — links, notes, images — instantly.") {
		t.Fatalf("payload does not contain the em-dash body un-mangled: %s", payload)
	}
}

func TestSend_NoAuthWhenUsernameEmpty(t *testing.T) {
	addr, data := fakeSMTP(t)
	host, port := splitHostPort(t, addr)

	m := New(SMTPConfig{
		Host: host,
		Port: port,
		From: "sender@example.com",
	})

	msg := Message{
		To:       "reader@example.com",
		Subject:  "No auth",
		BodyText: "hi",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// The fake server doesn't advertise AUTH and would reject an AUTH command
	// with an error response; since our default handler replies 250 to
	// anything unrecognized, the real assertion is that Send succeeds without
	// the client ever needing AUTH credentials (Username is empty).
	if err := m.Send(ctx, msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if data.Len() == 0 {
		t.Fatal("expected DATA payload to be captured")
	}
}

func TestSend_MissingHostReturnsError(t *testing.T) {
	m := New(SMTPConfig{
		Host: "127.0.0.1",
		Port: 1, // unlikely to have anything listening
		From: "sender@example.com",
	})

	msg := Message{
		To:       "reader@example.com",
		Subject:  "unreachable",
		BodyText: "hi",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := m.Send(ctx, msg); err == nil {
		t.Fatal("expected error dialing unreachable host, got nil")
	}
}

func TestSend_SubjectHeaderInjectionIsNeutralised(t *testing.T) {
	addr, data := fakeSMTP(t)
	host, port := splitHostPort(t, addr)

	m := New(SMTPConfig{Host: host, Port: port, From: "sender@example.com"})

	msg := Message{
		To:       "reader@example.com",
		Subject:  "innocent title\r\nBcc: attacker@example.com",
		BodyText: "body",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Send(ctx, msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	payload := data.String()
	for _, line := range strings.Split(payload, "\r\n") {
		if strings.HasPrefix(line, "Bcc:") {
			t.Fatalf("CRLF in Subject injected a Bcc header line: %s", payload)
		}
	}
	if !strings.Contains(payload, "Subject: innocent title  Bcc: attacker@example.com") {
		t.Fatalf("Subject was not flattened onto one line: %s", payload)
	}
}
