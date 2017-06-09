package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func TestMain(m *testing.M) {
	_, err := exec.LookPath("git")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(m.Run())
}

// TODO: be graceful
func TestRun(t *testing.T) {
	tests := []struct {
		args    []string
		wanterr bool
	}{
		// gits flags
		{
			args:    []string{"gits"},
			wanterr: true,
		},
		{
			args:    []string{"gits", "-version"},
			wanterr: false,
		},
		{
			args:    []string{"gits", "-list"},
			wanterr: false,
		},
		{
			args:    []string{"gits", `-git=""`, "version"},
			wanterr: true,
		},

		// git arguments
		{
			args:    []string{"gits", "version"},
			wanterr: false,
		},
		{
			args:    []string{"gits", "status", "--invalid--git--flags"},
			wanterr: true,
		},
		{
			args:    []string{"gits", "not impl"},
			wanterr: true,
		},
	}

	var s, errs string
	buf := bytes.NewBufferString(s)
	errbuf := bytes.NewBufferString(errs)
	for i, test := range tests {
		exitCode := run(buf, errbuf, nil, test.args)
		switch exitCode {
		case validExit:
			if test.wanterr {
				t.Errorf("t.Errorf [%d] expected error but nil", i)
			}
		case exitWithErr:
			if test.wanterr {
				t.Logf("t.Logf [%d] passed error: %+v", i, errbuf)
			} else {
				t.Errorf("t.Errorf [%d] err: %+v", i, errbuf)
			}
		}
		t.Logf("t.Logf [%d] outbuf: %+v", i, buf)
		buf.Reset()
		errbuf.Reset()
	}
}
