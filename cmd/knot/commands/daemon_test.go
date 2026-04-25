package commands

import (
	"errors"
	"net"
	"os"
	"syscall"
	"testing"
)

func TestIsDaemonNotRunningError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "os not exist",
			err:  os.ErrNotExist,
			want: true,
		},
		{
			name: "syscall enoent",
			err:  syscall.ENOENT,
			want: true,
		},
		{
			name: "syscall econnrefused",
			err:  syscall.ECONNREFUSED,
			want: true,
		},
		{
			name: "wrapped net dial not exist",
			err: &net.OpError{
				Op:  "dial",
				Net: "unix",
				Err: &os.SyscallError{Syscall: "connect", Err: syscall.ENOENT},
			},
			want: true,
		},
		{
			name: "wrapped net dial connection refused",
			err: &net.OpError{
				Op:  "dial",
				Net: "unix",
				Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED},
			},
			want: true,
		},
		{
			name: "other error",
			err:  errors.New("permission denied"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDaemonNotRunningError(tt.err)
			if got != tt.want {
				t.Fatalf("isDaemonNotRunningError() = %v, want %v", got, tt.want)
			}
		})
	}
}
