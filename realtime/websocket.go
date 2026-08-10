package realtime

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/saifsilver/goplusplus"
)

const magicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// Conn wraps a WebSocket connection over a raw net.Conn socket.
type Conn struct {
	netConn net.Conn
	reader  *bufio.Reader
}

// Upgrade performs a WebSocket handshake (101 Switching Protocols) and returns a WebSocket Conn.
func Upgrade(c *gpp.Context) (*Conn, error) {
	key := c.GetHeader("Sec-WebSocket-Key")
	if key == "" {
		return nil, gpp.ErrBadRequest("Missing Sec-WebSocket-Key header")
	}

	h := sha1.New()
	h.Write([]byte(key + magicGUID))
	acceptKey := base64.StdEncoding.EncodeToString(h.Sum(nil))

	hijacker, ok := c.Writer.(http.Hijacker)
	if !ok {
		return nil, errors.New("webserver does not support HTTP hijacking for WebSocket Upgrade")
	}

	netConn, bufrw, err := hijacker.Hijack()
	if err != nil {
		return nil, err
	}

	res := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey + "\r\n\r\n"

	_, err = netConn.Write([]byte(res))
	if err != nil {
		_ = netConn.Close()
		return nil, err
	}

	return &Conn{
		netConn: netConn,
		reader:  bufrw.Reader,
	}, nil
}

// ReadMessage reads a WebSocket text message frame.
func (c *Conn) ReadMessage() (string, error) {
	head := make([]byte, 2)
	if _, err := io.ReadFull(c.reader, head); err != nil {
		return "", err
	}

	masked := (head[1] & 0x80) != 0
	length := uint64(head[1] & 0x7f)

	if length == 126 {
		lenBuf := make([]byte, 2)
		if _, err := io.ReadFull(c.reader, lenBuf); err != nil {
			return "", err
		}
		length = uint64(lenBuf[0])<<8 | uint64(lenBuf[1])
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(c.reader, maskKey[:]); err != nil {
			return "", err
		}
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(c.reader, payload); err != nil {
		return "", err
	}

	if masked {
		for i := uint64(0); i < length; i++ {
			payload[i] ^= maskKey[i%4]
		}
	}

	return string(payload), nil
}

// WriteMessage sends a WebSocket text message frame.
func (c *Conn) WriteMessage(message string) error {
	data := []byte(message)
	length := len(data)

	var frame []byte
	frame = append(frame, 0x81) // Text frame (FIN bit set)

	if length <= 125 {
		frame = append(frame, byte(length))
	} else {
		frame = append(frame, 126, byte(length>>8), byte(length))
	}

	frame = append(frame, data...)
	_, err := c.netConn.Write(frame)
	return err
}

// Close terminates the WebSocket connection socket.
func (c *Conn) Close() error {
	return c.netConn.Close()
}
