package provisioning

import (
	"context"
	"fmt"
	"io"
	"os"
)

type FileUploader interface {
	Upload(context.Context, Target, []byte, string, string, os.FileMode) error
}

func (s SSHClient) Upload(ctx context.Context, target Target, privateKey []byte, localPath, remotePath string, mode os.FileMode) error {
	client, err := s.connect(ctx, target, privateKey)
	if err != nil {
		return err
	}
	defer client.Close()
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	go func() {
		defer stdin.Close()
		fmt.Fprintf(stdin, "C%04o %d payload\n", mode.Perm(), info.Size())
		_, _ = io.Copy(stdin, f)
		_, _ = io.WriteString(stdin, "\x00")
	}()
	if err := session.Run("scp -qt " + shellArg(remotePath)); err != nil {
		return err
	}
	return nil
}

func shellArg(s string) string { return "'" + s + "'" }
