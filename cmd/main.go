package main

import (
	"fmt"

	excesscommands "github.com/Flafl/DevOpsCore/internal/excessCommands/Nokia"
	"github.com/Flafl/DevOpsCore/internal/shell"
)

var creds = map[string]string{
	"username": "devops",
	"password": "Le@@M0DjkHIerR34S",
}

var sshPool = shell.NewConnectionPool(creds["username"], creds["password"])

func main() {
	fmt.Println(excesscommands.TotalInventory(sshPool))
}
