package shell

import (
	"context"
	"fmt"
	"strings"
)

func NocDataRecoverySendCommandUsingMethod(host, user, pass, vendor, method string, cmds ...string) (string, string, error) {
	return NocDataRecoverySendCommandUsingMethodContext(context.Background(), host, user, pass, vendor, method, cmds...)
}

func NocDataRecoverySendCommandUsingMethodContext(ctx context.Context, host, user, pass, vendor, method string, cmds ...string) (string, string, error) {
	host = strings.TrimSpace(host)
	vendor = strings.TrimSpace(vendor)
	if ctx == nil {
		ctx = context.Background()
	}

	switch strings.ToLower(strings.TrimSpace(method)) {
	case "", "auto":
		out, err := NocDataRecoverySSHSendCommandContext(ctx, host, user, pass, vendor, cmds...)
		if err == nil {
			return out, "ssh", nil
		}
		if ctx.Err() != nil {
			return "", "", ctx.Err()
		}

		telnetOut, telnetErr := NocDataRecoveryTelnetSendCommandContext(ctx, host, user, pass, vendor, cmds...)
		if telnetErr == nil {
			return telnetOut, "telnet", nil
		}
		return "", "", fmt.Errorf("recovery ssh failed: %v; recovery telnet failed: %w", err, telnetErr)
	case "ssh":
		out, err := NocDataRecoverySSHSendCommandContext(ctx, host, user, pass, vendor, cmds...)
		if err != nil {
			return "", "", err
		}
		return out, "ssh", nil
	case "telnet":
		out, err := NocDataRecoveryTelnetSendCommandContext(ctx, host, user, pass, vendor, cmds...)
		if err != nil {
			return "", "", err
		}
		return out, "telnet", nil
	default:
		return "", "", fmt.Errorf("unsupported NOC data recovery transport method %q", method)
	}
}

func NocDataRecoverySSHSendCommandContext(ctx context.Context, host, user, pass, vendor string, cmds ...string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "mikrotik":
		return MikrotikNocPassSSHContext(ctx, host, user, pass, cmds...)
	case "cisco", "nexus":
		profile, ok := vendorProfiles[strings.ToLower(strings.TrimSpace(vendor))]
		if !ok {
			return "", fmt.Errorf("unknown vendor %q", vendor)
		}
		sshUser := strings.TrimSpace(user) + profile.UsernameSuffix

		type result struct {
			output string
			err    error
		}
		ch := make(chan result, 1)
		go func() {
			out, err := ciscoLegacyRawSSH(host, sshUser, pass, vendor, profile, cmds...)
			ch <- result{output: out, err: err}
		}()

		select {
		case res := <-ch:
			return res.output, res.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	case "huawei":
		type result struct {
			output string
			err    error
		}
		ch := make(chan result, 1)
		go func() {
			out, err := huaweiWorkflowSSH(host, user, pass, cmds...)
			ch <- result{output: out, err: err}
		}()

		select {
		case res := <-ch:
			return res.output, res.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	default:
		return "", fmt.Errorf("unsupported recovery SSH vendor %q", vendor)
	}
}

func NocDataRecoveryTelnetSendCommandContext(ctx context.Context, host, user, pass, vendor string, cmds ...string) (string, error) {
	return NocDataTelnetSendCommandContext(ctx, host, user, pass, vendor, cmds...)
}
