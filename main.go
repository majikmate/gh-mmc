package main

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmd/factory"
	"github.com/majikmate/gh-mmc/cmd/root"
)

func main() {
	cmdFactory := factory.New("0.0.1")

	cmd := root.NewRootCmd(cmdFactory)
	err := cmd.Execute()

	if err != nil {
		fmt.Println(err)
	}
}
