/*
CLI tool for periodically backing up files. Intended mainly for backing up
save files for games where the developers think it's a good idea to randomly
lose your progress.

Requires Go. Use any of the following, depending on your preference:

	* Official download: https://go.dev/dl/.
	* MacOS: `brew install go`.
	* Linux: use your system's package manager.
	* Windows: `scoop install golang` (if you use Scoop).
	* Windows: `choco install golang` (if you use Chocolatey).

Before running, specify the source and destination path
by editing the constants `SRC_DIR` and `TAR_DIR` below.

To run, open a terminal, navigate to this directory, then:

	go run .

The makefile in the source directory is for development only.
*/
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mitranim/gg"
	"github.com/rjeczalik/notify"
)

var FLAGS = Flags{Config: `backup.json`}

type Flags struct {
	Config  string `json:"config"`
	Help    bool   `json:"help"`
	Verbose bool   `json:"verbose"`
}

type Config struct {
	CommonConfig
	Entries []Entry `json:"entries"`
}

type Entry struct {
	CommonConfig
	Input  string `json:"input"`
	Output string `json:"output"`
}

type CommonConfig struct {
	Debounce gg.Opt[Millisec] `json:"debounce"`
	Deadline gg.Opt[Millisec] `json:"deadline"`
	Limit    gg.Opt[uint64]   `json:"limit"`
}

type RunInput struct {
	Config  Config
	Entry   Entry
	Initial bool
}

const DEFAULT_DEBOUNCE Millisec = 1000
const DEFAULT_DEADLINE Millisec = 1000 * 10
const DEFAULT_LIMIT = 128

func main() {
	log.SetOutput(os.Stderr)
	flag.CommandLine.SetOutput(os.Stderr)
	flag.Usage = usage
	flag.BoolVar(&FLAGS.Help, `h`, FLAGS.Help, `print help and exit`)
	flag.BoolVar(&FLAGS.Verbose, `v`, FLAGS.Verbose, `verbose logging`)
	flag.StringVar(&FLAGS.Config, `c`, FLAGS.Config, `config file`)
	flag.Parse()

	if FLAGS.Help {
		usage()
		os.Exit(0)
		return
	}

	args := flag.Args()
	if len(args) > 0 {
		if args[0] == `help` {
			usage()
			os.Exit(0)
			return
		}

		fmt.Fprintf(os.Stderr, "unexpected arguments: %q\n", args)
		os.Exit(1)
		return
	}

	if FLAGS.Config == `` {
		fmt.Fprintln(os.Stderr, `missing path to config file`)
		os.Exit(1)
		return
	}

	if !gg.FileExists(FLAGS.Config) {
		fmt.Fprintf(os.Stderr, "missing config file %q\n", FLAGS.Config)
		os.Exit(1)
		return
	}

	events := make(chan notify.EventInfo, 1)
	watchConfig(FLAGS.Config, events)
	defer notify.Stop(events)

	ctx, cancel := context.WithCancel(context.Background())
	go run(ctx)

	for range events {
		if FLAGS.Verbose {
			log.Println(`restarting on config change`)
		}

		cancel()
		ctx, cancel = context.WithCancel(context.Background())
		go run(ctx)
	}
}

const HELP = `CLI tool for automatic file backups.
Watches specified input paths, detects changes,
and copies files to the specified output paths.

Input and output paths are specified via a JSON
configuration file. By default it's "backup.json"
in the current directory. You may specify another
path.

Example "backup.json":

  {
    "limit": 32,
    "entries": [
      {
        "input": "<file_or_directory_path>",
        "output": "<directory_path>"
      }
    ]
  }

The tool also watches its configuration file and
restarts on any changes to it.

Flags:

`

func usage() {
	fmt.Fprint(os.Stderr, HELP)
	flag.PrintDefaults()
}

/*
Watching a single file doesn't seem to work on Windows at the moment.
We report the error and proceed anyway, as this is non-critical.
Github issue: https://github.com/rjeczalik/notify/issues/225.
*/
func watchConfig(path string, events chan notify.EventInfo) {
	err := notify.Watch(path, events, notify.All)

	if err != nil {
		if FLAGS.Verbose {
			log.Printf(`unable to watch config file: %+v`, err)
		} else {
			log.Printf(`unable to watch config file: %v`, err)
		}
		return
	}

	if FLAGS.Verbose {
		log.Printf(`watching config file %q`, path)
	}
}

func readConfig() (out Config) {
	path := FLAGS.Config
	defer gg.Detailf(`unable to decode config file %q`, path)
	gg.JsonDecodeFile(path, &out)
	return
}

func run(ctx context.Context) {
	defer gg.RecWith(logErr)
	conf := readConfig()

	for _, entry := range conf.Entries {
		go runEntry(ctx, conf, entry)
	}
}

func runEntry(ctx context.Context, conf Config, entry Entry) {
	defer gg.RecWith(logErr)

	events := make(chan notify.EventInfo, 2)
	gg.Try(notify.Watch(filepath.Join(entry.Input, `...`), events, notify.All))
	defer notify.Stop(events)

	if FLAGS.Verbose {
		log.Printf(`watching %q`, entry.Input)
	}

	var run RunInput
	run.Initial = true
	run.Config = conf
	run.Entry = entry

	backup(run)
	run.Initial = false

outer:
	for {
		select {
		case <-ctx.Done():
			return

		case eve := <-events:
			if FLAGS.Verbose {
				log.Println(`FS event detected:`, eve)
			}

			debounce := run.GetDebounce().Duration()
			if debounce == 0 {
				backup(run)
				continue outer
			}

			var dead <-chan time.Time
			deadline := run.GetDeadline().Duration()
			if deadline != 0 {
				dead = time.After(deadline)
			}

			for {
				select {
				case <-ctx.Done():
					return
				case eve := <-events:
					if FLAGS.Verbose {
						log.Println(`FS event detected:`, eve)
					}
				case <-time.After(debounce):
					backup(run)
					continue outer
				case <-dead:
					backup(run)
					continue outer
				}
			}
		}
	}
}

func backup(run RunInput) {
	defer gg.RecWith(logErr)
	defer gg.Detailf(`failed to backup %q`, run.Entry.Input)

	inp := gg.ParseTo[IndexedName](run.Entry.Input)
	outs := gg.Sorted(relatedNames(run.Entry.Output, inp))
	prev := gg.Last(outs)
	defer gg.Ok(func() { cleanup(run, outs) })

	if run.Initial && gg.IsNotZero(prev) {
		name := prev.String()
		path := filepath.Join(run.Entry.Output, name)
		nextTime := maxModTime(run.Entry.Input)
		prevTime := maxModTime(path)
		if prevTime.After(nextTime) {
			if FLAGS.Verbose {
				fmt.Fprintf(os.Stderr, "backup %q is already up to date\n", path)
			}
			return
		}
	}

	next := gg.Or(prev, inp)
	next.Index = gg.Inc(next.Index) // Panics in case of overflow.

	path := filepath.Join(run.Entry.Output, next.String())
	copyRecursive(run.Entry.Input, path, run.Entry.Output)

	// For `cleanup`.
	outs = append(outs, next)

	if FLAGS.Verbose {
		fmt.Fprintf(os.Stderr, "backed up %q\n", path)
	}
}

func cleanup(run RunInput, outs []IndexedName) {
	limit := gg.NumConv[int](run.GetLimit())
	if limit <= 0 {
		return
	}

	for _, out := range gg.Take(outs, len(outs)-limit) {
		path := filepath.Join(run.Entry.Output, out.String())
		_ = os.RemoveAll(path)

		if FLAGS.Verbose {
			log.Printf(`deleted %q`, path)
		}
	}
}

func logErr(err error) {
	if err == nil {
		return
	}
	if FLAGS.Verbose {
		log.Printf(`%+v`, err)
	} else {
		log.Println(err)
	}
}

type Millisec uint64

func (self Millisec) Duration() time.Duration {
	return gg.Mul(gg.NumConv[time.Duration](self), time.Millisecond)
}

func (self *RunInput) GetDebounce() Millisec {
	return optGet(optCoalesce(self.Entry.Debounce, self.Config.Debounce), DEFAULT_DEBOUNCE)
}

func (self *RunInput) GetDeadline() Millisec {
	return optGet(optCoalesce(self.Entry.Deadline, self.Config.Deadline), DEFAULT_DEADLINE)
}

func (self *RunInput) GetLimit() uint64 {
	return optGet(optCoalesce(self.Entry.Limit, self.Config.Limit), DEFAULT_LIMIT)
}

func optCoalesce[A any](src ...gg.Opt[A]) gg.Opt[A] {
	return gg.Find(src, gg.Opt[A].IsNotNull)
}

func optGet[A any](src gg.Opt[A], def A) A {
	if src.Ok {
		return src.Val
	}
	return def
}

/*
Difference from `filepath.Ext`: a name that begins with a dot, like
`.gitignore`, is considered to be a name, without an extension.
Whereas `filepath.Ext` would consider it to have the extension
`.gitignore` with an empty name.
*/
func fileNameSplit(src string) (string, string) {
	name := filepath.Base(src)
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if base == `` {
		return ext, ``
	}
	return base, ext
}

// Difference from `os.ReadDir`: returns `[]string`, not `[]os.FileInfo`.
func readDir(path string) []string {
	defer gg.SkipOnly(isErrFileNotFound)
	file := gg.Try1(os.OpenFile(path, os.O_RDONLY, os.ModePerm))
	defer file.Close()
	return gg.Try1(file.Readdirnames(-1))
}

func isErrFileNotFound(err error) bool { return errors.Is(err, os.ErrNotExist) }

const INDEX_SEP = `_`

const INDEX_RADIX = 10

var INDEX_WIDTH = Index(math.MaxUint64).Width()

type Index uint64

func (self Index) String() string {
	missing := INDEX_WIDTH - self.Width()
	if missing <= 0 {
		return strconv.FormatUint(uint64(self), INDEX_RADIX)
	}

	buf := make(gg.Buf, INDEX_WIDTH)
	for ind := range gg.Iter(missing) {
		buf[ind] = '0'
	}
	strconv.AppendUint(buf[missing:missing], uint64(self), INDEX_RADIX)
	return buf.String()
}

func (self Index) Width() (out int) {
	if self == 0 {
		return 1
	}
	for self > 0 {
		out++
		self /= INDEX_RADIX
	}
	return
}

type IndexedName struct {
	Name  string
	Index Index
	Ext   string
}

func (self IndexedName) String() string {
	if self.Index == 0 {
		return self.Name + self.Ext
	}
	return self.Name + INDEX_SEP + self.Index.String() + self.Ext
}

func (self *IndexedName) UnmarshalText(src []byte) error {
	self.Decode(gg.ToString(src))
	return nil
}

func (self *IndexedName) Decode(src string) {
	name, ext := fileNameSplit(gg.ToString(src))
	if name == `` {
		self.Name = name
		self.Index = 0
		self.Ext = ext
		return
	}

	ind := strings.LastIndex(name, INDEX_SEP)
	if ind < 0 {
		self.Name = name
		self.Index = 0
		self.Ext = ext
		return
	}

	val, err := strconv.ParseUint(name[ind+len(INDEX_SEP):], INDEX_RADIX, 64)
	if err != nil {
		self.Name = name
		self.Index = 0
		self.Ext = ext
		return
	}

	self.Name = name[:ind]
	self.Index = Index(val)
	self.Ext = ext
}

func (self IndexedName) Related(tar IndexedName) bool {
	return self.Name == tar.Name && self.Ext == tar.Ext
}

func (self IndexedName) Less(tar IndexedName) bool {
	return self.Index < tar.Index
}

/*
Note: despite its name, `filepath.WalkDir` also supports walking a single file.
This function should work for both directory backups and single file backups.
*/
func maxModTime(src string) (out time.Time) {
	gg.Try(filepath.WalkDir(
		src,
		func(_ string, src fs.DirEntry, _ error) error {
			if src == nil {
				return nil
			}

			info, _ := src.Info()
			if info == nil {
				return nil
			}

			val := info.ModTime()
			if val.After(out) {
				out = val
			}
			return nil
		},
	))
	return
}

func relatedNames(dir string, inp IndexedName) (out []IndexedName) {
	out = gg.Map(readDir(dir), gg.ParseTo[IndexedName, string])
	out = gg.Filter(out, inp.Related)
	return
}

func copyRecursive(src, tar, dir string) {
	if gg.Try1(os.Stat(src)).IsDir() {
		copyDirRecursive(src, tar)
	} else {
		gg.Try(os.MkdirAll(dir, os.ModePerm))
		copyFile(src, tar)
	}
}

func copyDirRecursive(srcDir, tarDir string) {
	for _, name := range readDir(srcDir) {
		copyRecursive(
			filepath.Join(srcDir, name),
			filepath.Join(tarDir, name),
			tarDir,
		)
	}
}

func copyFile(srcPath, tarPath string) {
	src := gg.Try1(os.OpenFile(srcPath, os.O_RDONLY, os.ModePerm))
	defer src.Close() // Ignore error.

	out := gg.Try1(os.Create(tarPath))
	defer gg.Close(out) // Do not ignore error.

	gg.Try1(io.Copy(out, src))
}
