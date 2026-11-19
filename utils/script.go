package utils

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func FindScript(nameWithoutExt string) string {
	ext := ".sh"
	if runtime.GOOS == "windows" {
		ext = ".bat"
	}
	scriptName := nameWithoutExt + ext

	if _, err := os.Stat(scriptName); err == nil {
		return scriptName
	}

	execDir := filepath.Dir(os.Args[0])
	execScript := filepath.Join(execDir, scriptName)
	if _, err := os.Stat(execScript); err == nil {
		return execScript
	}

	return ""
}

// CopySH executes script for copying/deploying releases
func CopySH() {
	scriptPath := FindScript("copy")
	if scriptPath == "" {
		return
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd.exe", "/C", scriptPath)
	} else {
		cmd = exec.Command("/bin/sh", scriptPath)
	}
	logOut, err := cmd.CombinedOutput()
	if err != nil {
		log.Println("Error copy releases:", err)
	}
	output := strings.TrimSpace(string(logOut))
	if len(output) > 0 {
		log.Println(output)
	}
}
