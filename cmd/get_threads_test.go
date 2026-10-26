package cmd

import (
	"io"
	"strings"
	"testing"
)

func TestThreadListValidation(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--root", "--branch"}, "mutually exclusive"},
		{[]string{"id", "--answer", "a"}, "mutually exclusive"},
		{[]string{"--answer", ""}, "must not be empty"},
		{[]string{"--user", " "}, "must not be empty"},
		{[]string{"--page", "0"}, "positive"},
		{[]string{"--limit", "101"}, "between 1 and 100"},
		{[]string{"--limit", "0"}, "between 1 and 100"},
		{[]string{"--output", "yaml"}, "invalid output format"},
		{[]string{"id", "--user", "me"}, "cannot be combined"},
		{[]string{"--answer", "a", "--page", "1"}, "cannot be combined"},
		{[]string{"id", "--root=false"}, "cannot be combined"},
		{[]string{"--order", "time"}, "requires --public"},
		{[]string{"--public", "--order", "bogus"}, "must be time"},
		{[]string{"--public", "--user", "me"}, "cannot be combined"},
		{[]string{"--public", "--limit", "101"}, "between 1 and 100"},
		{[]string{"--public", "--page", "2"}, "does not apply"},
		{[]string{"--public", "--order", "upvalue", "--limit", "5"}, "apply only"},
		{[]string{"--public", "--order", "upvalue", "--before", "x"}, "apply only"},
		{[]string{"--before", "x"}, "requires --public"},
		{[]string{"--public", "--root"}, "cannot be combined"},
		{[]string{"--public", "--answer", "a"}, "cannot be combined"},
		{[]string{"--public", "--order", "upvalue", "--page", "0"}, "positive"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			command := newGetThreadsCommand()
			command.SetOut(io.Discard)
			command.SetErr(io.Discard)
			command.SetArgs(tc.args)
			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}
