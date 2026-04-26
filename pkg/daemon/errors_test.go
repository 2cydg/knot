package daemon

import (
	"errors"
	"net"
	"os"
	"syscall"
	"testing"
)

func TestIsNotRunningError(t *testing.T) {
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
			name: "wrapped net dial connection refused",
			err: &net.OpError{
				Op:  "dial",
				Net: "unix",
				Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED},
			},
			want: true,
		},
		{
			name: "wrapped windows winsock connection refused",
			err: &net.OpError{
				Op:  "dial",
				Net: "unix",
				Err: &os.SyscallError{Syscall: "connect", Err: syscall.Errno(10061)},
			},
			want: true,
		},
		{
			name: "windows unix socket actively refused",
			err:  errors.New("dial unix D:\\_cache\\TEMP\\knot-S-1-5-21-2678045406-545865077-1760447680-1001\\knot.sock: connect: No connection could be made because the target machine actively refused it."),
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
			got := IsNotRunningError(tt.err)
			if got != tt.want {
				t.Fatalf("IsNotRunningError() = %v, want %v", got, tt.want)
			}
		})
	}
}
