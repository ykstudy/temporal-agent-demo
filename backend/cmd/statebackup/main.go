// Offline single-writer state copy and isolated restore validation.
package main

import (
	"agentstudio/internal/agent"
	"flag"
	"fmt"
	"os"
)

func main() {
	source := flag.String("source", "", "stopped source data directory")
	destination := flag.String("destination", "", "new disjoint copy directory")
	verify := flag.String("verify", "", "isolated copy to validate and recover; may complete its pending transactions")
	mode := flag.String("execution-mode", "local", "local or temporal, no Temporal connection is opened")
	flag.Parse()
	var info agent.BackupStateInfo
	var err error
	if *verify != "" && *source == "" && *destination == "" {
		info, err = agent.VerifyOfflineCopy(*verify)
		if err == nil {
			cfg := agent.Config{DataDir: *verify, Mode: *mode}
			s, e := agent.New(cfg)
			err = e
			if e == nil {
				s.Close()
			}
		}
	} else if *verify == "" && *source != "" && *destination != "" {
		info, err = agent.CopyOfflineState(*source, *destination)
	} else {
		fmt.Fprintln(os.Stderr, "use --source and --destination, or --verify for an isolated copy")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "state copy/recovery rejected:", err)
		os.Exit(1)
	}
	fmt.Printf("verified files=%d bytes=%d manifest_sha256=%s\n", info.Files, info.Bytes, info.ManifestSHA256)
}
