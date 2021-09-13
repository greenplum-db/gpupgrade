// Copyright (c) 2017-2021 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

/*
Package daemon provides utilities for programs that need to fork themselves into
the background (for instance, a persistent server). Its primary function,
MakeDaemonizable, adds the --daemonize option to a cobra.Command.

Usage

These utilities are designed for use with cobra.Command. Clients of the package
need to do five things:

1. Provide a boolean variable (we'll call it shouldDaemonize, though you can
name it whatever you want) which will store whether the current process is the
child server process. (If you're familiar with the fork() syscall, think of this
as its zero return value.)

2. Call MakeDaemonizable(), which adds the --daemonize option to the command and
sets up the use of the shouldDaemonize flag.

3. During the command's Run() function, if the shouldDaemonize flag has been
set, call Daemonize() after the server has fully initialized and is ready to
receive requests. This will disconnect the standard streams and signal the
original process to exit.

4. Handle the ErrSuccessfullyDaemonized pseudo-error if it is returned from the
command's Execute() method. This is an indication that the child has correctly
started, and the parent should now exit with a zero exit code.

5. Write to standard error from the child server process if, and only if, it is
about to terminate without calling Daemonize(). (For an explanation of this
limitation, see "Implementation" below.)

All stdout/stderr contents emitted by the server during initialization will be
bubbled up to the caller of the original binary, so just use those streams as
you normally would.

Here's an example:

	var shouldDaemonize bool
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			err := listenOnServerPort()
			if err != nil {
				// Cobra will write an error message to stderr for us.
				return err
			}

			if shouldDaemonize {
				fmt.Println("started successfully; now daemonizing")
				daemon.Daemonize() // disconnect standard streams
			}

			return serverMainLoop()
		},
	}

	daemon.MakeDaemonizable(cmd, &shouldDaemonize) // add --daemonize

	err := cmd.Execute()
	if err != nil {
		if err == daemon.ErrSuccessfullyDaemonized {
			// not actually an "error", just exit cleanly
		} else {
			// handle actual error
			...
		}
	}

Telling this example server to daemonize is easy:

	$ example_server --daemonize
	started successfully; now daemonizing

	$ # at this point, the exit code indicates success/failure of server startup

We can also execute synchronously, just like before:

	$ example_server
	# ... serves until we Ctrl-C out

MakeDaemonizable() preempts the PreRun/PreRunE functions of the cobra.Command it
is passed. If --daemonize is set, the parent process will only execute the
PersistentPreRun[E] steps; ErrSuccessfullyDaemonized will be returned from the
PreRun step, which will stop further steps from executing. In the child process,
and during execution without the --daemonize option, all steps run as usual.

Motivation

This package helps solve a common race condition when initializing systems that
have servers and dependent clients. Consider the following setup, where a client
starts a server that it will then make a request to:

	Client
	|
	| ------- start server ------> Server
	| ------- make request ------> | ?
	|                              |

This situation has a race -- there's no guarantee that the server will have
initialized to the point that it is even listening for requests by the time the
client makes one. A common workaround is to tell the client to sleep, to "give
the server time to come up":

	Client
	|
	| ------- start server ------> Server
	| sleeping...                  |
	| sleeping...                  |  initializing...
	| sleeping...                  |
	| sleeping...                  |- listening
	| sleeping...                  |
	| ------- make request ------> |
	|                              |

Not only does this not solve the actual race, but it tends to inject unnecessary
latency as the authors try to tune the sleep time to the slowest known
environment. A better solution is to periodically poll the server during
startup, but a robust client then also needs some way to poll for early/abnormal
server exit so that it doesn't wait too long for a server that never actually
started.

For a distributed system, these problems come with the domain, but for cases in
which the client and server are on the same machine, we can do better. To get
rid of the race, we need the server to be able to signal the child as soon as it
is ready to listen, before it enters its main loop:

	Client
	|
	| ------- start server ------> Server
	| waiting for signal...        |
	|                              |  initializing...
	|                              |
	| <------------- ready ------- |- listening
	| ------- make request ------> |
	| <---------- response ------- |
	|                              |

This way, the server is guaranteed to either 1) be listening or 2) have
terminated unexpectedly by the time the client's request is made, and error
handling is greatly simplified.

Implementation

The daemon package implements this signaling with standard POSIX streams and
process exit codes, so no special client libraries are required. An intermediate
"server parent" process is introduced so that the client can easily wait for
that process to exit before continuing. (The server process itself is supposed
to persist after creation, so the client can't wait for it to exit.) If the
parent process exits with a non-zero code, then the server has failed to launch;
otherwise, it has been forked into the background and is ready to receive
requests.

Child-to-parent signaling is implemented using the child's stdout/stderr
streams. When both are closed (either deliberately or through an abnormal exit),
the parent will continue. If any stderr contents are found, the parent will then
wait for an exit code from the child. Otherwise, the parent will exit cleanly
and allow the child to continue serving.

(This magic use of stderr does introduce one complication: the child server
process must write to stderr *if and only if* it terminates abnormally before it
is able to process requests.)

The full system looks like this:

	Client
	|
	| ---- execute server -----> Server Parent
	| waiting for process...     |
	|                            | ---- start child -------> Server
	|                            | waiting for child...      |
	|                            |                           |  initializing...
	|                            |                           |
	|                            | <----- close streams ---- |- listening
	| <---------- exit code ---- x parent exits              |
	|                                                        |
	| ----- make request ----------------------------------> |
	| <-------------------------------------- response ----- |
	|                                                        |

*/
package daemon

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// daemonizableCommand is the interface required by waitForDaemon; exec.Command
// implements it. It's factored out explicitly to enable testing.
type daemonizableCommand interface {
	StdoutPipe() (io.ReadCloser, error)
	StderrPipe() (io.ReadCloser, error)
	Start() error
	Wait() error
}

// waitForDaemon connects the stdout/err pipes from the passed command to the
// given io.Writers, then starts the command and waits for the streams to be
// disconnected. If any stderr content is found, waitForDaemon will call Wait on
// the command and return the resulting error; otherwise, it returns
// successfully as soon as the pipes are closed.
//
// The passed timeout is used only when waiting for the child to exit after it
// prints to stderr. This function will wait indefinitely for the standard
// streams to close.
func waitForDaemon(command daemonizableCommand, output, errput io.Writer, timeout time.Duration) error {
	// Open up pipes from the child process and start it.
	stderr, err := command.StderrPipe()
	if err != nil {
		return err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}

	err = command.Start()
	if err != nil {
		return err
	}

	// Spin up two goroutines to copy the entire contents of both pipes to our
	// output Writers.
	var wg sync.WaitGroup
	done := make(chan error, 2)
	connectPipe := func(out io.Writer, in io.Reader, written *bool) {
		defer wg.Done()
		num, err := io.Copy(out, in)
		if written != nil {
			*written = (num > 0)
		}
		if err != nil {
			done <- fmt.Errorf("Could not copy from child pipe: %v", err)
		}
	}

	var hadStderr bool
	wg.Add(2)
	go connectPipe(output, stdout, nil)
	go connectPipe(errput, stderr, &hadStderr)
	wg.Wait()

	if len(done) > 0 {
		return <-done
	}

	// If we got stderr, wait for the process to exit.
	if hadStderr {
		var timer <-chan time.Time
		if timeout > 0 {
			timer = time.After(timeout)
		}

		result := make(chan error, 1)
		go func() {
			result <- command.Wait()
		}()

		select {
		case err := <-result:
			return err
		case <-timer:
			// XXX We leave the Wait()ing goroutine to die. Is that okay? We
			// don't want to kill the process; it might be functional, and
			// that's for the user to decide...
			return fmt.Errorf("the daemon process reported an error but did not immediately exit. Review the logs before continuing")
		}
	}

	return nil
}

// createChildProcess is a pseudo-fork(). If the child indicates an error and
// exits early, this function doesn't return and instead calls os.Exit with the
// same error code.
func createChildProcess() error {
	// Go's use of goroutines means we can't actually fork() the process.
	// Instead, spin up an identical copy of this process, but replace the
	// --daemonize option with --daemon so that we know it's the child. Then
	// wait for that child to close its streams before continuing.
	daemonArgs := make([]string, 0)
	for _, arg := range os.Args[1:] {
		if arg == "--daemonize" {
			arg = "--daemon"
		}
		daemonArgs = append(daemonArgs, arg)
	}

	// Create our child process.
	command := exec.Command(os.Args[0], daemonArgs...)
	// TODO: what's a good timeout?
	err := waitForDaemon(command, os.Stdout, os.Stderr, 2*time.Second)

	if err != nil {
		exitError, ok := err.(*exec.ExitError)
		if ok {
			// Exit with the same code as the child, if we can figure it out.
			code := 1

			status, ok := exitError.Sys().(syscall.WaitStatus)
			if ok {
				code = status.ExitStatus()
			}

			os.Exit(code)
		}

		// Otherwise, deal with the error normally.
	}

	return err
}

// ErrSuccessfullyDaemonized is returned from a cobra.Command's Execute() method
// when the --daemonize option is passed to the executable and the child process
// indicates that it has started successfully.
var ErrSuccessfullyDaemonized = fmt.Errorf("child process daemonized successfully")

// MakeDaemonizable adds the --daemonize option to  the passed cobra.Command.
// The boolean pointed to by shouldDaemonize will be set to true in the child
// server process; it is a signal that the child should call Daemonize() when it
// is ready.
func MakeDaemonizable(cmd *cobra.Command, shouldDaemonize *bool) {
	// Set up our flags.
	//
	// --daemonize tells us that we should create a new child, wait for it to
	// start, and then exit.
	//
	// --daemon tells us that we *are* the newly created child. We should start
	// and then disconnect our standard streams. (This option shouldn't be
	// passed by an end user; we only use it internally when starting the
	// child.)
	var forkChild bool
	cmd.Flags().BoolVar(&forkChild, "daemonize", false, "start hub in the background")
	cmd.Flags().BoolVar(shouldDaemonize, "daemon", false, "disconnect standard streams (internal option; use --daemonize instead)")
	cmd.Flags().MarkHidden("daemon") //nolint

	// Don't destroy any prerun functions stored by the user. We'll call them
	// after our prerun, if we don't daemonize.
	var savedPreRunE func(cmd *cobra.Command, args []string) error
	var savedPreRun func(cmd *cobra.Command, args []string)

	if cmd.PreRunE != nil {
		savedPreRunE = cmd.PreRunE
	} else if cmd.PreRun != nil {
		savedPreRun = cmd.PreRun
	}

	// Set up our prerun function.
	//
	// If the user passed --daemonize, this is where we'll "fork" the new child
	// and wait for it to disconnect its standard streams. We also sanity check
	// to make sure no one passed --daemon from the command line. This function
	// will not return if the child process exits with an error code; in that
	// case, we'll exit immediately with the same code.
	//
	// If --daemonize isn't passed, this defers to the original PreRun[E]
	// function, if one existed.
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if *shouldDaemonize && term.IsTerminal(int(os.Stdout.Fd())) {
			// Shouldn't be using --daemon from the command line.
			return fmt.Errorf("--daemon is an internal option (did you mean --daemonize?)")
		}

		if forkChild {
			err := createChildProcess()

			if err == nil {
				// We return a dummy error so that Cobra doesn't continue
				// running command steps. Also silence Cobra's error reporting
				// for this case.
				err = ErrSuccessfullyDaemonized
				cmd.SilenceErrors = true
				cmd.SilenceUsage = true
			}

			return err
		}

		/* If we didn't daemonize, call the user's PreRun[E] (if one exists). */
		if savedPreRunE != nil {
			return savedPreRunE(cmd, args)
		} else if savedPreRun != nil {
			savedPreRun(cmd, args)
		}

		return nil
	}
}

// Daemonize disconnects the standard output and error streams of the current
// process, signaling to the parent that it has started successfully. Use this
// only when the shouldDaemonize flag is set, after your server has fully
// initialized and is ready to handle requests.
//
// Don't call Daemonize when terminating abnormally during server startup.
// Instead, write to stderr and exit with a nonzero exit code. The parent will
// pick them up and exit with the same error.
func Daemonize() {
	// TODO: Research daemonize to see what else may need to be done for the
	// child process to safely detach from the parent
	os.Stderr.Close()
	os.Stdout.Close()
}
