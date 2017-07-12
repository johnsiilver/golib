package server

// SessionType describes a particular session the client would send the server.
type SessionType string

// A set of session request types.  We only support a subset of the full amount of request types.
const (
  // Shell requests that the user's default shell be started.  More information on this request type can be found here:
  // https://www.ietf.org/rfc/rfc4254.txt section 6.5
  Shell SessionType = "shell"

  // Exec requests that the server start exeuction of a command.  More information on this request type can be found here:
  // https://www.ietf.org/rfc/rfc4254.txt section 6.5
  Exec SessionType = "exec"

  // Subsystem requests execution of a pred-defined subsystem on the server .  More information on this request type can be found here:
  // https://www.ietf.org/rfc/rfc4254.txt section 6.5
  Subsystem SessionType = "subsystem"
)

// SessionHandler provides a mapping of SessionType's to functions that handles those sessions.
type SessionHandlers map[SessionType]SessionHandler

// SessionHandler handles a single request comming off the channel.  Do not close the channel,
// as our wrapper handles closing of the channel when there are no more requests or when you return an error.
// Auth can be nil, in which case it does not need to be checked.
type SessionHandler func(channel ssh.Channel, *request ssh.Request, serverConn *ssh.ServerConn, auth Authorizer) error

// HandleExecWithCmd provides a SessionHandler that handles SessionType Exec by taking the command
// and funnelling it to the underlying operating system for processing.
func HandleExecWithCmd(channel ssh.Channel, serverConn *ServerConn)
  var reqCmd struct{ Text string }
  if err := ssh.Unmarshal(req.Payload, &reqCmd); err != nil {
      panic(err)
  }

  log.Infof("server: got command: %q\n", reqCmd.Text)
  cmd := exec.Command(reqCmd.Text)
  cmd.Stdout = channel
  cmd.Stderr = channel.Stderr()
  err := cmd.Run()
  if err != nil {
      panic(err)
  }

  if req.WantReply {
      // no special payload for this reply
      req.Reply(true, nil) // or false if the command failed to run successfully
  }
  if _, err := channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0}); err != nil {
      panic(err)
  }

  if err := channel.Close(); err != nil {
      panic(err)
  }
}
