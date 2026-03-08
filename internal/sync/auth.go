package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

var errSSHPasswordCancelled = errors.New("ssh password prompt cancelled")

type authStateKey struct{}

type authState struct {
	mu          sync.Mutex
	sshPassword string
}

var (
	askpassScriptOnce sync.Once
	askpassScriptPath string
	askpassScriptErr  error
)

func withAuthState(ctx context.Context) context.Context {
	return context.WithValue(ctx, authStateKey{}, &authState{})
}

func authStateFromContext(ctx context.Context) *authState {
	state, _ := ctx.Value(authStateKey{}).(*authState)
	return state
}

func (s *authState) Password() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sshPassword
}

func (s *authState) SetPassword(password string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.sshPassword = password
	s.mu.Unlock()
}

func (s *authState) ClearPassword() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.sshPassword = ""
	s.mu.Unlock()
}

func runSSHCommandWithPasswordPrompt(
	ctx context.Context,
	eventCh chan<- Event,
	step int,
	target string,
	log func(string),
	attempt func(extraEnv []string, batchMode bool) error,
) error {
	if err := attempt(nil, true); err == nil || !shouldPromptForSSHPassword(err) {
		return err
	}

	state := authStateFromContext(ctx)
	if cached := state.Password(); cached != "" {
		log("  retrying with cached SSH password")
		err := retryWithSSHPassword(state, cached, attempt)
		if err == nil || !shouldPromptForSSHPassword(err) {
			return err
		}
		state.ClearPassword()
	}

	password, err := requestSSHPassword(ctx, eventCh, step, target)
	if err != nil {
		return err
	}
	state.SetPassword(password)
	log("  retrying with provided SSH password")
	return retryWithSSHPassword(state, password, attempt)
}

func retryWithSSHPassword(
	state *authState,
	password string,
	attempt func(extraEnv []string, batchMode bool) error,
) error {
	extraEnv, err := askpassEnv(password)
	if err != nil {
		return err
	}
	err = attempt(extraEnv, false)
	if shouldPromptForSSHPassword(err) {
		state.ClearPassword()
	}
	return err
}

func requestSSHPassword(ctx context.Context, eventCh chan<- Event, step int, target string) (string, error) {
	replyCh := make(chan AuthReply, 1)
	prompt := fmt.Sprintf("SSH password for %s", target)
	if !sendEvent(ctx, eventCh, Event{Type: EvAuthRequest, Step: step, Message: prompt, AuthReplyCh: replyCh}) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return "", errSSHPasswordCancelled
	}

	select {
	case reply := <-replyCh:
		if reply.Cancel {
			return "", errSSHPasswordCancelled
		}
		return reply.Password, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func shouldPromptForSSHPassword(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	markers := []string{
		"permission denied",
		"authentication failed",
		"password:",
		"number of password prompts exceeded",
		"publickey,password",
		"publickey,gssapi",
		"keyboard-interactive",
	}
	for _, marker := range markers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func askpassEnv(password string) ([]string, error) {
	script, err := askpassScript()
	if err != nil {
		return nil, err
	}
	return []string{
		"DISPLAY=sitesync:0",
		"SSH_ASKPASS=" + script,
		"SSH_ASKPASS_REQUIRE=force",
		"SITESYNC_SSH_PASSWORD=" + password,
	}, nil
}

func askpassScript() (string, error) {
	askpassScriptOnce.Do(func() {
		f, err := os.CreateTemp("", "sitesync-askpass-*")
		if err != nil {
			askpassScriptErr = fmt.Errorf("create askpass helper: %w", err)
			return
		}
		defer f.Close()

		content := "#!/bin/sh\nprintf '%s\\n' \"$SITESYNC_SSH_PASSWORD\"\n"
		if _, err := f.WriteString(content); err != nil {
			askpassScriptErr = fmt.Errorf("write askpass helper: %w", err)
			return
		}
		if err := os.Chmod(f.Name(), 0700); err != nil {
			askpassScriptErr = fmt.Errorf("chmod askpass helper: %w", err)
			return
		}
		askpassScriptPath = f.Name()
	})

	if askpassScriptErr != nil {
		return "", askpassScriptErr
	}
	return askpassScriptPath, nil
}

func sshOptionValues(batchMode bool) []string {
	options := []string{
		"PreferredAuthentications=publickey,password,keyboard-interactive",
	}
	if batchMode {
		options = append(options,
			"BatchMode=yes",
			"NumberOfPasswordPrompts=0",
		)
	} else {
		options = append(options,
			"BatchMode=no",
			"NumberOfPasswordPrompts=1",
		)
	}
	return options
}

func sshArgs(port int, batchMode bool) []string {
	args := []string{"-p", fmt.Sprintf("%d", port)}
	for _, option := range sshOptionValues(batchMode) {
		args = append(args, "-o", option)
	}
	return args
}

func scpArgs(port int, batchMode bool) []string {
	args := []string{"-P", fmt.Sprintf("%d", port)}
	for _, option := range sshOptionValues(batchMode) {
		args = append(args, "-o", option)
	}
	return args
}

func rsyncSSHCommand(port int, batchMode bool) string {
	parts := append([]string{"ssh"}, sshArgs(port, batchMode)...)
	return strings.Join(parts, " ")
}

func commandEnv(extraEnv []string) []string {
	if len(extraEnv) == 0 {
		return nil
	}
	return append(os.Environ(), extraEnv...)
}
