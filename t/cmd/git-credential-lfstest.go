//go:build testtools
// +build testtools

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	commands = map[string]func(){
		"get":   fill,
		"store": log,
		"erase": log,
	}

	delim    = '\n'
	credsDir = ""
)

func init() {
	if len(credsDir) == 0 {
		credsDir = os.Getenv("CREDSDIR")
	}
}

func main() {
	if argsize := len(os.Args); argsize != 2 {
		fmt.Fprintf(os.Stderr, "wrong number of args: %d\n", argsize)
		os.Exit(1)
	}

	arg := os.Args[1]
	cmd := commands[arg]

	if cmd == nil {
		fmt.Fprintf(os.Stderr, "bad cmd: %s\n", arg)
		os.Exit(1)
	}

	cmd()
}

func fill() {
	scanner := bufio.NewScanner(os.Stdin)
	creds := map[string][]string{}
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "bad line: %s\n", line)
			os.Exit(1)
		}

		fmt.Fprintf(os.Stderr, "CREDS RECV: %s\n", line)
		if _, ok := creds[parts[0]]; ok {
			creds[parts[0]] = append(creds[parts[0]], strings.TrimSpace(parts[1]))
		} else {
			creds[parts[0]] = []string{strings.TrimSpace(parts[1])}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "reading standard input: %v", err)
		os.Exit(1)
	}

	hostPieces := strings.SplitN(firstEntryForKey(creds, "host"), ":", 2)
	authtype, user, cred, err := credsForHostAndPath(hostPieces[0], firstEntryForKey(creds, "path"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	capas := discoverCapabilities(creds)

	switch authtype {
	case "skip":
	case "":
		if _, ok := creds["username"]; !ok {
			creds["username"] = []string{user}
		}

		if _, ok := creds["password"]; !ok {
			creds["password"] = []string{cred}
		}
	default:
		if _, ok := capas["authtype"]; ok {
			if _, ok := creds["authtype"]; !ok {
				creds["authtype"] = []string{authtype}
			}

			if _, ok := creds["credential"]; !ok {
				creds["credential"] = []string{cred}
			}
		}
	}

	mode := os.Getenv("LFS_TEST_CREDS_WWWAUTH")
	wwwauth := firstEntryForKey(creds, "wwwauth[]")
	if mode == "required" && !strings.HasPrefix(wwwauth, "Basic ") {
		fmt.Fprintf(os.Stderr, "Missing required 'wwwauth[]' key in credentials\n")
		os.Exit(1)
	} else if mode == "forbidden" && wwwauth != "" {
		fmt.Fprintf(os.Stderr, "Unexpected 'wwwauth[]' key in credentials\n")
		os.Exit(1)
	}
	delete(creds, "wwwauth[]")

	// Send capabilities first to all for one-pass parsing.
	for _, entry := range creds["capability[]"] {
		key := "capability[]"
		fmt.Fprintf(os.Stderr, "CREDS SEND: %s=%s\n", key, entry)
		fmt.Fprintf(os.Stdout, "%s=%s\n", key, entry)
	}
	for key, value := range creds {
		if key == "capability[]" {
			continue
		}
		for _, entry := range value {
			fmt.Fprintf(os.Stderr, "CREDS SEND: %s=%s\n", key, entry)
			fmt.Fprintf(os.Stdout, "%s=%s\n", key, entry)
		}
	}
}

func discoverCapabilities(creds map[string][]string) map[string]struct{} {
	capas := make(map[string]struct{})
	supportedCapas := map[string]struct{}{
		"authtype": struct{}{},
	}
	capasToSend := []string{}
	for _, capa := range creds["capability[]"] {
		capas[capa] = struct{}{}
		// Only pass on capabilities we support.
		if _, ok := supportedCapas[capa]; ok {
			capasToSend = append(capasToSend, capa)
		}
	}
	creds["capability[]"] = capasToSend
	return capas
}

func credsForHostAndPath(host, path string) (string, string, string, error) {
	var hostFilename string

	// We need hostFilename to end in a slash so that our credentials all
	// end up in the same directory.  credsDir will come in from the
	// testsuite with a slash, but filepath.Join will strip it off if host
	// is empty, such as when we have a file:/// or cert:/// URL.
	if host != "" {
		hostFilename = filepath.Join(credsDir, host)
	} else {
		hostFilename = credsDir
	}

	if len(path) > 0 {
		pathFilename := fmt.Sprintf("%s--%s", hostFilename, strings.Replace(path, "/", "-", -1))
		authtype, u, cred, err := credsFromFilename(pathFilename)
		if err == nil {
			return authtype, u, cred, err
		}
	}

	return credsFromFilename(hostFilename)
}

func credsFromFilename(file string) (string, string, string, error) {
	credential, err := os.ReadFile(file)
	if err != nil {
		return "", "", "", fmt.Errorf("Error opening %q: %s", file, err)
	}
	credsPieces := strings.SplitN(strings.TrimSpace(string(credential)), ":", 3)
	if len(credsPieces) != 3 {
		return "", "", "", fmt.Errorf("Invalid data %q while reading %q", string(credential), file)
	}
	return credsPieces[0], credsPieces[1], credsPieces[2], nil
}

func log() {
	fmt.Fprintf(os.Stderr, "CREDS received command: %s (ignored)\n", os.Args[1])
}

func firstEntryForKey(input map[string][]string, key string) string {
	if val, ok := input[key]; ok && len(val) > 0 {
		return val[0]
	}
	return ""
}
