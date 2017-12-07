// git wrapper
// Quick Usage:
//   `gits -template > /path/a/watchlist.json`
// edit gits.json, append your repository
// after append
//   `gits -conf-dir=/path/a -conf=watchlist.json status`
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const version = "0.0.2"

const (
	validExit = iota
	exitWithErr
)

type option struct {
	version  bool
	template bool
	list     bool
	edit     bool

	showConfPath bool
	showConfDirs bool
	showConfList bool

	confNew string // create configuration file to confDir

	// TODO: add flags?
	// logfile string // specify output logfile
	// execute string // specify commands: -e [command]

	watch   string /// add watch to conf
	unwatch string /// delte watch in conf

	git     string
	match   string
	conf    string
	confdir string
	sync    bool
	timeout time.Duration
}

// repository walker
// use: git --git-dir=/path/to/work/.git --work-tree=/path/to/work
// TODO: sync consider to join structure of subcmd
func gitWalker(git *subcmd, runOnSync bool, wl *watchList, args []string) []error {
	// work on current directory
	// need it?
	if wl.Map == nil || len(wl.Map) == 0 {
		msg := fmt.Sprintf("not found git repositories:\n\twork on current directory\n")
		git.WriteErrString(msg)
		if err := git.run("", args); err != nil {
			return []error{err}
		}
		return nil
	}

	var (
		errs         []error
		mux          = new(sync.Mutex)
		wg           = new(sync.WaitGroup)
		argsWithRepo []string
	)

	var do func(string)
	if runOnSync {
		do = func(key string) {
			premsg := fmt.Sprintf("\n[%s]\n", key)
			if err := git.run(premsg, argsWithRepo); err != nil {
				errs = append(errs, fmt.Errorf("[%s]:%+v", key, err))
			}
		}
	} else {
		do = func(key string) {
			wg.Add(1)
			go func(key string) {
				defer wg.Done()
				premsg := fmt.Sprintf("\n[%s]\n", key)
				if err := git.run(premsg, argsWithRepo); err != nil {
					mux.Lock()
					errs = append(errs, fmt.Errorf("[%s]:%+v", key, err))
					mux.Unlock()
				}
			}(key)
		}
	}

	for key, repoInfo := range wl.Map {
		argsWithRepo = append(
			[]string{"--git-dir=" + repoInfo.Gitdir,
				"--work-tree=" + repoInfo.Workdir},
			args...,
		)
		do(key)
	}
	wg.Wait()
	return errs
}

func editConf(w, errw io.Writer, r io.Reader, editor, confpath string) error {
	if info, err := os.Stat(confpath); err != nil {
		return err
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not regular file", confpath)
	}
	sub := newSubcmd(w, errw, r, editor, 0)
	return sub.run("", []string{confpath})
}

// TODO: split file for configuration file path?

func run(w io.Writer, errw io.Writer, r io.Reader, args []string) int {
	opt := option{}
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)
	flags.SetOutput(errw)

	// one shot
	flags.BoolVar(&opt.version, "version", false, "")
	flags.BoolVar(&opt.template, "template", false, "output the template of watchlist")
	flags.BoolVar(&opt.list, "list", false, "list of accept first argument and repository")
	flags.BoolVar(&opt.edit, "edit", false, "open conf on your editor(default:"+DefEditor+")")
	flags.BoolVar(&opt.sync, "sync", false, "run on sync")

	flags.BoolVar(&opt.showConfPath, "conf-path", false, "show default conf path")
	flags.BoolVar(&opt.showConfDirs, "candidate-dirs", false, "show candidate conf directories")
	flags.BoolVar(&opt.showConfList, "conf-list", false, "show configuration set list")

	flags.StringVar(&opt.confNew, "conf-new", "", "generate new configuration file to conf directory")

	// modify conf
	flags.StringVar(&opt.watch, "watch", "", "add watching repository to conf")
	flags.StringVar(&opt.unwatch, "unwatch", "", "remove watching repository in conf")

	// setting
	flags.StringVar(&opt.git, "git", "git", "command name of git or full path")
	flags.StringVar(&opt.match, "match", "", "specify target repositories")

	flags.StringVar(&opt.conf, "conf", DefConfName, "accept base name or full path, to json format watchlist")
	flags.StringVar(&opt.conf, "c", DefConfName, "alias of [-conf]")

	flags.StringVar(&opt.confdir, "conf-dir", DefConfDir, "specify conf directory")
	flags.DurationVar(&opt.timeout, "timeout", 0, "set timeout for running git, 0 means no limit")
	flags.Parse(args[1:])

	/// TODO: reconsider confs
	var confpath string
	if opt.conf != "" {
		if filepath.IsAbs(opt.conf) {
			confpath = opt.conf
		} else {
			confpath = filepath.Join(opt.confdir, filepath.Base(opt.conf))
		}
	}

	wc := newWatchConf(confpath)

	if confpath != "" {
		var err error
		wc.wl, err = readWatchList(wc.path)
		if err != nil {
			fmt.Fprintln(errw, err)
			return exitWithErr
		}
	}
	if opt.match != "" {
		for key := range wc.wl.Map {
			matched, err := filepath.Match(opt.match, key)
			if err != nil {
				fmt.Fprintln(errw, err)
				return exitWithErr
			}
			if matched {
				continue
			}
			delete(wc.wl.Map, key)
		}
	}

	// one shot
	if flags.NArg() == 0 {
		switch {
		case opt.version:
			fmt.Fprintf(w, "version %s\n", version)
			return validExit
		case opt.showConfPath:
			fmt.Fprintln(w, confpath)
			return validExit
		case opt.showConfDirs:
			fmt.Fprintln(w, strings.Join(DefConfDirList, "\n"))
			return validExit
		case opt.showConfList:
			confList, err := wc.getConfList()
			if err != nil {
				fmt.Fprintln(errw, err)
				return exitWithErr
			}
			fmt.Fprintln(w, strings.Join(confList, "\n"))
			return validExit
		case opt.confNew != "":
			mkpath := filepath.Join(DefConfDir, filepath.Base(opt.confNew))
			if err := createConf(mkpath); err != nil {
				fmt.Fprintln(errw, err)
				return exitWithErr
			}
			fmt.Fprintln(w, "configuration file was written: "+mkpath)
			return validExit
		case opt.template:
			if err := template(w); err != nil {
				fmt.Fprintln(errw, err)
				return exitWithErr
			}
			return validExit
		case opt.list:
			fmt.Fprintf(w, "conf:[%s]\n%s\n", confpath, wc.wl)
			return validExit
		case opt.edit:
			if err := editConf(w, errw, r, DefEditor, confpath); err != nil {
				fmt.Fprintln(errw, err)
				return exitWithErr
			}
			return validExit
		case opt.watch != "":
			key, err := wc.watch(opt.watch)
			if err != nil {
				fmt.Fprintln(errw, err)
				return exitWithErr
			}
			fmt.Fprintf(w, "conf:[%s]\n%s\nappended [%s]\n", wc.path, wc.wl, key)
			return validExit
		case opt.unwatch != "":
			key, err := wc.unwatch(opt.unwatch)
			if err != nil {
				fmt.Fprintln(errw, err)
				return exitWithErr
			}
			fmt.Fprintf(w, "conf:[%s]\n%s\nremoved [%s]\n", wc.path, wc.wl, key)
			return validExit
		default:
			flags.Usage()
			return exitWithErr
		}
	}

	if !wc.wl.isAllow(flags.Arg(0)) {
		fmt.Fprintf(errw, "Configuration file path:\n\t[%s]\n%s\n", confpath, wc.wl)
		fmt.Fprintf(errw, "This argument is not allowed: %+v\n", flags.Args())
		return exitWithErr
	}

	git := newSubcmd(w, errw, r, opt.git, opt.timeout)
	if errs := gitWalker(git, opt.sync, wc.wl, flags.Args()); errs != nil {
		fmt.Fprintln(errw, "---------- found error ----------")
		fmt.Fprintln(errw, errs)
		return exitWithErr
	}
	return validExit
}

func main() {
	os.Exit(run(os.Stdout, os.Stderr, os.Stdin, os.Args))
}
