// Package server provides a simple way to create a custom SSH server.
package server

import (
  "golang.org/x/crypto/ssh"
	"golang.org/x/net/context"
)

// Authenication provides a method for authenticating a user via a password or SSH key.
type Authentication interface {
  Authenticate(passKey interface{}) (*sshPermissions, error)
}

// authentication authenicates a password or ssh.PublicKey against a UserPass or Key type for access to the server.
func authentication(auth interface{}, passKey interface{}) (*sshPermissions, error) {
  switch a := auth.(type) {
  case UserPass:
    p, ok := passKey.([]byte])
    if !ok {
      return nil, fmt.Errorf("auth was type UserPass, but passKey was not []byte, was %T", passKey)
    }

    if p != a.Password {
      return nil, errors.New("password was incorrect")
    }
  case Key:
    k, ok := passKey.(ssh.PublicKey)
    if !ok {
      return nil, fmt.Errorf("auth was type Key, but passKey was not ssh.PublicKey, was %T", passKey)
    }

    // TODO(jdoak): Add key authentication.
  default:
    return nil, fmt.Errorf("passed auth is not a type we support: %T", auth)
  }
}

// UserPass is used to store password and permission information when enforcing permissions via username/password.
// However, it is suggested to public key authentication via Key{} instead.
type UserPass struct {
  // Password is the password to authenticate this user against.  It is stronly encouraged that the
  // client pass a SHA256 hash and that this string is the SHA256 hash and not the actual password.
  // Not doing this endangers the password store if retrieved by a third-party.
  Password []byte

  // Permissions is the SSH permissions for the user.  This must be enforced by your client application.
  // Please see: https://godoc.org/golang.org/x/crypto/ssh#Permissions for more information.
  Permissions *ssh.Permissions
}

func (u *UserPass) Authenticate(passKey interface{}) (*sshPermissions, error) {
  return authentication(u, passKey)
}

// Key is used to store PublicKey and permission information when enforcing permissions via Public Key authentication.
type Key struct {
  // Key represents the SSH key associated with the user.
  Key ssh.PublicKey

  // Permissions is the SSH permissions for the user.  This must be enforced by your client application.
  // Please see: https://godoc.org/golang.org/x/crypto/ssh#Permissions for more information.
  Permissions *ssh.Permissions
}

func (k *Key) Authenticate(passKey interface{}) (*sshPermissions, error) {
  return authentication(u, passKey)
}

// AuthenicateMetaData is a function that takes in an SSH's connection metadata and returns
// an error if the connection should not be authenticated.
type AuthenicateMetaData func(m ConnMetadata) error

// Option provides optional arguments to the New() constructor.
type Option func(s *SSH)

// AuthMetaData provides a custom function for authenticating a connection based upon constraints such as what the remote address is.
func AuthMetaData(f AuthenicateMetaData) Option {
    return func(s *SSH) {
      s.authMetaData = f
    }
}

// CertChecker provides a method for checking the validity of certs presented for authentication.
// If this is not passed, no certs may be used fo authentication.
// See https://github.com/golang/crypto/blob/0c565bf13221fb55497d7ae2bb95694db1fd1bff/ssh/client_auth_test.go#L39 for an example.
func CertChecker(chk ssh.CertChecker) Option {
  return func(s *SSH) {
    s.certChecker = chk
  }
}

// Authorization provides authorization for a user on some input.  This can be provided for authorization levels that cannot be
// achieved via the ssh.Permissions type.
type Authorization interface {
  Authorized(user string, sessionType SessionType, input interface{}) bool
}

// Authorizer provides for a customer authorizer for a sessionType.
func Authorizer(sessionType SessionType, auth Authorization) Option {
  return func(s *SSH) {
      s.authorizer[sessionType] = auth
  }
}

// SSH provides a new SSH server.
type SSH struct {
  serverConfig *ssh.ServerConfig
  authUserPass map[string]UserPass
  authKey map[string]Key
  authMetaData AuthenicateMetaData
  certChecker ssh*CertChecker
  authorizers map[sessionType] Authorization

  running chan bool
  listener net.Listener

  handlers SessionHandlers
}

// New constructes a new SSH instance. "auth" provides a mapping from user names to Authenication methods.
// If "auth" is set to nil, no authentication is required (NOT RECOMMENDED).
func New(keyFilePath string, auth map[string]Authentication, opts ...Option) (*SSH, error) {
  s := &SH{
    authUserPass: make(map[string]UserPass),
    authKey: make(map[string]Key),
  }

  for _, opt := range opts {
    opt(s)
  }

  s.serverConfig := &ssh.ServerConfig{
    PasswordCallback: s.passwordCallback,
    KeyboardInteractiveCallback: s.keyboardInteractiveCallback,
    PublicKeyCallback: s.publicKeyCallback,
  }

  if auth != nil {
    for u, a := range auth {
      switch t := a.(type) {
      case UserPass:
        s.authUserPass[u] = t
      case Key:
        s.authKey[u] = t
      default:
        return nil, fmt.Errorf("auth had unsupported Authentication type: %T", t)
      }

    }
  }else{
    s.serverConfig.NoClientAuth = true
    log.Warningf("server is running with no client authentication!!!!")
  }

  privateBytes, err := ioutil.ReadFile(keyFilePath)
  if err != nil {
    return nil, fmt.Errorf("Failed to load private key: %s", err)
  }

  private, err := ssh.ParsePrivateKey(privateBytes)
  if err != nil {
    return nil, fmt.Errorf("Failed to parse private key: %s", err)
  }

  s.serverConfig.AddHostKey(private)
  return s, nil
}

// Start starts the SSH server.  Not thread-safe.
func (s *SSH) Start(addr string) error {
  var err error
  // Once a ServerConfig has been configured, connections can be accepted.
  s.listener, err = net.Listen("tcp", addr)
  if err != nil {
    return fmt.Errorf("failed to listen on %q: %s", addr, err)
  }

  // Accept all connections
  log.Infof("Listening at %q...", addr)
  s.running = make(chan bool)
  go func() {
      for {
    		tcpConn, err := s.listener.Accept()
    		if err != nil {
          select {
          case <-running:
            log.Infof("told to stop running ssh server")
            return
          default:
            // Keep going.
          }
    			log.Errorf("Failed to accept incoming connection (%s)", err)
    			continue
    		}

    		// Before use, a handshake must be performed on the incoming net.Conn.
    		sshConn, chans, reqs, err := ssh.NewServerConn(tcpConn, s.serverConfig)
    		if err != nil {
    			log.Errorf("Failed to handshake (%s)", err)
    			continue
    		}

    		log.Infof("New SSH connection from %s (%s)", sshConn.RemoteAddr(), sshConn.ClientVersion())
    		// Discard all global out-of-band Requests
    		go ssh.DiscardRequests(reqs)

    		// Accept all channels
    		go s.handleChannels(chans, sshConn)
    	}
    }
  }()

  return nil
}

// Stop stops the SSH server.  If you attenpt to stop a non-running server, this will panic.
// Not thread-safe.
func (s *SSH) Stop() error {
  close(s.running)
  return s.listener.Close()
}

// ServerConfig returns the *ssh.ServerConfig for custom tweaking purposes.  This is only valid before Start() is called.
// Changing this at any other time has undetermined behavior.  Not thread-safe.
func (s *SSH) ServerConfig() *ssh.ServerConfig {
  return s.serverConfig
}

// passwordCallback implements ssh.ServerConfig.PasswordCallback.
func (s *SSH) passwordCallback(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
  if s.authMetaData != nil {
    if err := s.authMetaData(c); err != nil {
      return nil, err
    }
  }

  rec, ok := s.authUserPass[c.User()]
  if !ok {
    return nil, fmt.Errorf("authenication failed, user %q not found", c.User())
  }

  if bytes.Compare(rec.Password, pass) != 0 {
    return nil, fmt.Errorf("authenication failed for user %q", c.User())
  }
  return rec.Permissions, nil
}

// keyboardInteractiveCallback implements ssh.ServerConfig.KeyboardInteractiveCallback.
func (s *SSH) keyboardInteractiveCallback func(conn ConnMetadata, client KeyboardInteractiveChallenge) (*Permissions, error) {
			ans, err := challenge(
        c.User(),
				"",
				[]string{"password:"},
				[]bool{false}
      )
			if err != nil {
				return nil, err
			}

      return s.authUserPass[c.User].Permissions, nil
}

// challenge implements ssh.KeyboardInteractiveChallenge by asking one question and expecting one answer. The answer
// is checked against the user's password.
func (s *SSH) challenge(user string, instruction string, questions []string, echos []bool) ([]string, error) {
  if len(questions) != 1 {
    panic("keyboard interactive should only be setup to do password auth")
  }

  var answers []string
  v, ok := s.authUserPass[user]; ok {
	 return []string{v}, nil
  }

  log.Infof("authorization failed: user %q unknown", user)
  return nil, fmt.Errorf("keyboard interactive authorization failed")
}

// publicKeyCallback implements ssh.PublicKeyCallback.
func (s *SSH) publicKeyCallback func(conn ConnMetadata, key PublicKey) (*Permissions, error) {
  if s.certChecker == nil {
    return failCertCheck
  }
  return s.certChecker.Authenticate
}

// failCertCheck authomatically fails any attempts to authenicate with a cert because the user did not pass
// the CertChecker option to New().
func (s *SSH) failCertCheck(conn ConnMetadata, key PublicKey) (*Permissions, error) {
  return nil, fmt.Errorf("no certifiate check was provided on the server, certs cannot be used for authentication")
}

// handleChannel spins off goroutines for each channel that comes in.
func (s *SSH) handleChannels(chans <-chan ssh.NewChannel, serverConn *ssh.ServerConn) {
	// Service the incoming Channel channel in go routine
	for newChannel := range chans {
		go handleChannel(newChannel, serverConn)
	}
}

var (
  exitCode0 = []byte{0, 0, 0, 0}
  exitCode1 = []byte{0, 0, 0, 1}
)

// handleChannel handles all requests that come in on a channel.
func (s *SSH) handleChannel(newChannel ssh.NewChannel, serverConn *ServerConn) {
	// Since we're handling a shell, we expect a
	// channel type of "session". The also describes
	// "x11", "direct-tcpip" and "forwarded-tcpip"
	// channel types.
	if t := newChannel.ChannelType(); t != "session" {
    log.Errorf("channel type was not 'session', was %q", t)
		newChannel.Reject(ssh.UnknownChannelType, fmt.Sprintf("unknown channel type: %s", t))
		return
	}

	// At this point, we have the opportunity to reject the client's
	// request for another logical connection
	channel, requests, err := newChannel.Accept()
	if err != nil {
		log.Errorf("Could not accept channel (%s)", err)
		return
	}

	go func() {
    defer func(){
      if err := channel.Close(); err != nil {
        log.Infof("error closing SSH channel: %s", err)
      }
    }()

		for req := range requests {
      handler, ok := s.handlers[SessionType(req.Type)]
      if !ok {
        log.Infof("SSH server does not support request type %q", req.Type)
        req.Reply(false, nil)
        channel.SendRequest("exit-status", false, exitCode1)
        continue
      }

      if err := handler(channel, req, serverConn); err != nil {
        log.Error(err)
        req.Reply(false, nil)
        channel.SendRequest("exit-status", false, exitCode1)
      }

      channel.SendRequest("exit-status", false, exitCode0)
		}
	}()
}
