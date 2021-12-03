package lime

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"sync"
)

const DefaultReadLimit int64 = 8192 * 1024

type TCPTransport struct {
	ReadLimit     int64       // ReadLimit defines the limit for buffered data in read operations.
	TraceWriter   TraceWriter // TraceWriter sets the trace writer for tracing connection envelopes
	conn          net.Conn
	encoder       *json.Encoder
	decoder       *json.Decoder
	limitedReader io.LimitedReader
	// TLSConfig The configuration for TLS session encryption
	TLSConfig  *tls.Config
	encryption SessionEncryption
	server     bool
}

// DialTcp opens a TCP  transport connection with the specified Uri.
func DialTcp(ctx context.Context, addr net.Addr, tls *tls.Config) (*TCPTransport, error) {
	if addr.Network() != "tcp" {
		return nil, errors.New("address network should be tcp")
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, addr.Network(), addr.String())
	if err != nil {
		return nil, err
	}

	t := TCPTransport{
		TLSConfig: tls,
	}

	t.setConn(conn)
	t.encryption = SessionEncryptionNone
	return &t, nil
}

func (t *TCPTransport) GetSupportedCompression() []SessionCompression {
	return []SessionCompression{SessionCompressionNone}
}

func (t *TCPTransport) GetCompression() SessionCompression {
	return SessionCompressionNone
}

func (t *TCPTransport) SetCompression(_ context.Context, c SessionCompression) error {
	return fmt.Errorf("compression '%v' is not supported", c)
}

func (t *TCPTransport) GetSupportedEncryption() []SessionEncryption {
	return []SessionEncryption{SessionEncryptionNone, SessionEncryptionTLS}
}

func (t *TCPTransport) GetEncryption() SessionEncryption {
	return t.encryption
}

func (t *TCPTransport) SetEncryption(ctx context.Context, e SessionEncryption) error {
	if e == t.encryption {
		return nil
	}

	if e == SessionEncryptionNone {
		return errors.New("cannot downgrade from tls to none encryption")
	}

	if e == SessionEncryptionTLS && t.TLSConfig == nil {
		return errors.New("tls config must be defined")
	}

	var tlsConn *tls.Conn

	// https://github.com/FluuxIO/go-xmpp/blob/master/xmpp_transport.go#L80
	if t.server {
		tlsConn = tls.Server(t.conn, t.TLSConfig)
	} else {
		tlsConn = tls.Client(t.conn, t.TLSConfig)
	}

	deadline, _ := ctx.Deadline() // Use the deadline zero value if ctx has no deadline defined
	if err := tlsConn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	if err := tlsConn.SetReadDeadline(deadline); err != nil {
		return err
	}

	// We convert existing connection to TLS
	if err := tlsConn.Handshake(); err != nil {
		return err
	}

	t.setConn(tlsConn)
	t.encryption = SessionEncryptionTLS
	return nil
}

func (t *TCPTransport) Send(ctx context.Context, e Envelope) error {
	if ctx == nil {
		panic("nil context")
	}

	if e == nil || reflect.ValueOf(e).IsNil() {
		panic("nil envelope")
	}

	if err := t.ensureOpen(); err != nil {
		return err
	}

	// Sets the timeout for the next write operation
	deadline, _ := ctx.Deadline()
	if err := t.conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	// TODO: Handle context <-Done() signal
	// TODO: Encode writes a new line after each entry, how we can avoid this?
	return t.encoder.Encode(e)
}

func (t *TCPTransport) Receive(ctx context.Context) (Envelope, error) {
	if ctx == nil {
		panic("nil context")
	}

	if err := t.ensureOpen(); err != nil {
		return nil, err
	}

	// Sets the timeout for the next read operation
	deadline, _ := ctx.Deadline()
	if err := t.conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}

	var raw rawEnvelope

	// TODO: Handle context <-Done() signal
	if err := t.decoder.Decode(&raw); err != nil {
		return nil, err
	}

	// Reset the read limit
	t.limitedReader.N = t.ReadLimit

	return raw.ToEnvelope()
}

func (t *TCPTransport) Close() error {
	if err := t.ensureOpen(); err != nil {
		return err
	}

	err := t.conn.Close()
	t.conn = nil
	return err
}

func (t *TCPTransport) IsConnected() bool {
	return t.conn != nil
}

func (t *TCPTransport) LocalAddr() net.Addr {
	if t.conn == nil {
		return nil
	}
	return t.conn.LocalAddr()
}

func (t *TCPTransport) RemoteAddr() net.Addr {
	if t.conn == nil {
		return nil
	}
	return t.conn.RemoteAddr()
}

func (t *TCPTransport) setConn(conn net.Conn) {
	t.conn = conn

	var writer io.Writer = t.conn
	var reader io.Reader = t.conn

	// Configure the trace writer, if defined
	tw := t.TraceWriter
	if tw != nil {
		writer = io.MultiWriter(writer, *tw.SendWriter())
		reader = io.TeeReader(reader, *tw.ReceiveWriter())
	}

	// Sets the encoder to be used for sending envelopes
	t.encoder = json.NewEncoder(writer)

	if t.ReadLimit == 0 {
		t.ReadLimit = DefaultReadLimit
	}

	// Using a LimitedReader to avoid the connection be
	// flooded with a large JSON which may cause
	// high memory usage.
	t.limitedReader = io.LimitedReader{
		R: reader,
		N: t.ReadLimit,
	}
	t.decoder = json.NewDecoder(&t.limitedReader)
}

func (t *TCPTransport) ensureOpen() error {
	if t.conn == nil {
		return errors.New("transport is not open")
	}

	return nil
}

type TCPTransportListener struct {
	ReadLimit   int64       // ReadLimit defines the limit for buffered data in read operations.
	TraceWriter TraceWriter // TraceWriter sets the trace writer for tracing connection envelopes
	TLSConfig   *tls.Config
	listener    net.Listener
	mu          sync.Mutex
}

func (t *TCPTransportListener) Listen(ctx context.Context, addr net.Addr) error {
	if addr.Network() != "tcp" {
		return errors.New("address network should be tcp")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.listener != nil {
		return errors.New("tcp listener is already started")
	}

	var lc net.ListenConfig
	l, err := lc.Listen(ctx, "tcp", addr.String())
	if err != nil {
		return err
	}

	t.listener = l
	return nil
}

func (t *TCPTransportListener) Accept(ctx context.Context) (Transport, error) {
	if t.listener == nil {
		return nil, errors.New("tcp listener is not started")
	}

	err := ctx.Err()
	if err != nil {
		return nil, err
	}

	conn, err := t.listener.Accept()
	if err != nil {
		return nil, err
	}

	transport := TCPTransport{
		TLSConfig:  t.TLSConfig,
		encryption: SessionEncryptionNone,
	}
	transport.server = true
	transport.ReadLimit = t.ReadLimit

	transport.setConn(conn)

	return &transport, nil
}

func (t *TCPTransportListener) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.listener == nil {
		return errors.New("tcp listener is not started")
	}

	err := t.listener.Close()
	t.listener = nil

	return err
}
