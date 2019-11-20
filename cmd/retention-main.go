/*
 * MinIO Client (C) 2019 MinIO, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/minio/cli"
	json "github.com/minio/mc/pkg/colorjson"
	"github.com/minio/mc/pkg/console"
	"github.com/minio/mc/pkg/probe"
	minio "github.com/minio/minio-go/v6"
)

var retentionCmd = cli.Command{
	Name:   "retention",
	Usage:  "set object retention for objects with a given prefix",
	Action: mainRetention,
	Before: setGlobalsFromContext,
	Flags:  globalFlags,
	CustomHelpTemplate: `NAME:
  {{.HelpName}} - {{.Usage}}

USAGE:
  {{.HelpName}} [FLAGS] TARGET [GOVERNANCE | COMPLIANCE] [VALIDITY]

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}
VALIDITY:
  This argument must be formatted like Nd or Ny where 'd' denotes days and 'y' denotes years e.g. 10d, 3y.

EXAMPLES:
   1. Set object retention for objects in a given prefix
     $ {{.HelpName}} myminio/mybucket/prefix GOVERNANCE 30d
`,
}

// Structured message depending on the type of console.
type retentionCmdMessage struct {
	Enabled  string               `json:"enabled"`
	Mode     *minio.RetentionMode `json:"mode"`
	Validity *string              `json:"validity"`
	Status   string               `json:"status"`
}

// Colorized message for console printing.
func (m retentionCmdMessage) String() string {
	if m.Mode == nil {
		return fmt.Sprintf("No mode is enabled")
	}

	return fmt.Sprintf("%s mode is enabled for %s", console.Colorize("Mode", *m.Mode), console.Colorize("Validity", *m.Validity))
}

// JSON'ified message for scripting.
func (m retentionCmdMessage) JSON() string {
	msgBytes, e := json.MarshalIndent(m, "", " ")
	fatalIf(probe.NewError(e), "Unable to marshal into JSON.")
	return string(msgBytes)
}

// doList - list all entities inside a folder.
func setRetention(urlStr string, mode *minio.RetentionMode, validity *uint, unit *minio.ValidityUnit) error {
	client, err := newClient(urlStr)
	if err != nil {
		fatalIf(err.Trace(), "Cannot parse the provided url.")
	}
	clnt, ok := client.(*s3Client)
	if !ok {
		fatalIf(errDummy().Trace(), "The provided url doesn't point to a S3 server.")
	}
	prefixPath := clnt.GetURL().Path
	separator := string(clnt.GetURL().Separator)
	if !strings.HasSuffix(prefixPath, separator) {
		prefixPath = prefixPath[:strings.LastIndex(prefixPath, separator)+1]
	}
	var cErr error
	for content := range clnt.List(true, false, DirNone) {
		if content.Err != nil {
			errorIf(content.Err.Trace(clnt.GetURL().String()), "Unable to list folder.")
			cErr = exitStatus(globalErrorExitStatus) // Set the exit status.
			continue
		}
		probeErr := clnt.PutObjectRetention(content.URL.Path, mode, validity, unit)
		fatalIf(probeErr, "Cannot set object retention on %s.", content.URL.Path)
		fmt.Println("success", content.URL.Path)
	}
	return cErr
}

// main for retention command.
func mainRetention(ctx *cli.Context) error {
	console.SetColor("Mode", color.New(color.FgCyan, color.Bold))
	console.SetColor("Validity", color.New(color.FgYellow))

	// Parse encryption keys per command.
	_, err := getEncKeys(ctx)
	fatalIf(err, "Unable to parse encryption keys.")

	// lock specific flags.
	clearLock := ctx.Bool("clear")

	args := ctx.Args()

	var urlStr string
	var mode *minio.RetentionMode
	var validity *uint
	var unit *minio.ValidityUnit

	switch l := len(args); l {
	case 3:
		urlStr = args[0]
		if clearLock {
			fatalIf(probe.NewError(errors.New("invalid argument")), "clear flag must be passed with target alone")
		}

		m := minio.RetentionMode(args[1])
		if !m.IsValid() {
			fatalIf(probe.NewError(errors.New("invalid argument")), "invalid retention mode '%v'", m)
		}

		mode = &m

		validityStr := args[2]
		unitStr := string(validityStr[len(validityStr)-1])

		validityStr = validityStr[:len(validityStr)-1]
		ui64, err := strconv.ParseUint(validityStr, 10, 64)
		if err != nil {
			fatalIf(probe.NewError(errors.New("invalid argument")), "invalid validity '%v'", args[2])
		}
		u := uint(ui64)
		validity = &u

		switch unitStr {
		case "d", "D":
			d := minio.Days
			unit = &d
		case "y", "Y":
			y := minio.Years
			unit = &y
		default:
			fatalIf(probe.NewError(errors.New("invalid argument")), "invalid validity format '%v'", args[2])
		}
	default:
		cli.ShowCommandHelpAndExit(ctx, "retention", 1)
	}
	return setRetention(urlStr, mode, validity, unit)
}
