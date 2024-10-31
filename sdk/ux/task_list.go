package ux

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dario.cat/mergo"
	"github.com/fatih/color"
	"github.com/wbreza/azd-extensions/sdk/common"
)

type TaskListConfig struct {
	// The writer to use for output (default: os.Stdout)
	Writer             io.Writer
	MaxConcurrentAsync int
	SuccessStyle       string
	ErrorStyle         string
	WarningStyle       string
	RunningStyle       string
	SkippedStyle       string
	PendingStyle       string
}

var DefaultTaskListConfig TaskListConfig = TaskListConfig{
	Writer:             os.Stdout,
	MaxConcurrentAsync: 5,

	SuccessStyle: color.GreenString("(✔) Done "),
	ErrorStyle:   color.RedString("(x) Error "),
	WarningStyle: color.YellowString("(!) Warning "),
	RunningStyle: color.CyanString("(-) Running "),
	SkippedStyle: color.HiBlackString("(-) Skipped "),
	PendingStyle: color.HiBlackString("(o) Pending "),
}

type TaskList struct {
	canvas    Canvas
	waitGroup sync.WaitGroup
	config    *TaskListConfig
	allTasks  []*Task
	syncTasks []*Task // Queue for synchronous tasks

	completed      int32
	syncMutex      sync.Mutex // Mutex to handle sync task queue safely
	errorMuxtex    sync.Mutex // Mutex to handle errors slice safely
	asyncSemaphore chan struct{}
	errors         []error
}

type TaskOptions struct {
	Title  string
	Action func() (TaskState, error)
	Async  bool
}

type Task struct {
	Title     string
	Action    func() (TaskState, error)
	State     TaskState
	Error     error
	startTime *time.Time
	endTime   *time.Time
}

type TaskState int

const (
	Pending TaskState = iota
	Running
	Skipped
	Warning
	Error
	Success
)

func NewTaskList(config *TaskListConfig) *TaskList {
	mergedConfig := TaskListConfig{}

	if config == nil {
		config = &TaskListConfig{}
	}

	if err := mergo.Merge(&mergedConfig, config, mergo.WithoutDereference); err != nil {
		panic(err)
	}

	if err := mergo.Merge(&mergedConfig, DefaultTaskListConfig, mergo.WithoutDereference); err != nil {
		panic(err)
	}

	return &TaskList{
		config:         &mergedConfig,
		waitGroup:      sync.WaitGroup{},
		allTasks:       []*Task{},
		syncTasks:      []*Task{},
		syncMutex:      sync.Mutex{},
		errorMuxtex:    sync.Mutex{},
		completed:      0,
		asyncSemaphore: make(chan struct{}, mergedConfig.MaxConcurrentAsync),
		errors:         []error{},
	}
}

func (t *TaskList) WithCanvas(canvas Canvas) Visual {
	t.canvas = canvas
	return t
}

// Run executes all async tasks first and then runs queued sync tasks sequentially.
func (t *TaskList) Run() error {
	if t.canvas == nil {
		t.canvas = NewCanvas(t)
	}

	if err := t.canvas.Run(); err != nil {
		return err
	}

	go func() {
		for {
			if t.Completed() {
				break
			}

			time.Sleep(1 * time.Second)
			t.Update()
		}
	}()

	// Wait for all async tasks to complete
	t.waitGroup.Wait()
	// Run sync tasks after async tasks are completed
	t.runSyncTasks()
	t.Update()

	if len(t.errors) > 0 {
		return errors.Join(t.errors...)
	}

	return nil
}

// AddTask adds a task to the task list and manages async/sync execution.
func (t *TaskList) AddTask(options TaskOptions) *TaskList {
	task := &Task{
		Title:  options.Title,
		Action: options.Action,
		State:  Pending,
	}

	// Differentiate between async and sync tasks
	if options.Async {
		t.addAsyncTask(task)
	} else {
		t.addSyncTask(task)
	}

	t.allTasks = append(t.allTasks, task)

	return t
}

// Completed checks if all async tasks are complete.
func (t *TaskList) Completed() bool {
	return int(t.completed) == len(t.allTasks)
}

func (t *TaskList) Update() error {
	if t.canvas == nil {
		t.canvas = NewCanvas(t)
	}

	return t.canvas.Update()
}

func (t *TaskList) Render(printer Printer) error {
	printer.Fprintln()

	for _, task := range t.allTasks {
		endTime := time.Now()
		if task.endTime != nil {
			endTime = *task.endTime
		}

		var elapsedText string
		if task.startTime != nil {
			elapsed := endTime.Sub(*task.startTime)
			elapsedText = color.HiBlackString("(%s)", durationAsText(elapsed))
		}

		var errorDescription string
		if task.Error != nil {
			var detailedErr *common.DetailedError
			if errors.As(task.Error, &detailedErr) {
				errorDescription = detailedErr.Description()
			} else {
				errorDescription = task.Error.Error()
			}
		}

		switch task.State {
		case Pending:
			printer.Fprintf("%s %s\n", color.HiBlackString(t.config.PendingStyle), task.Title)
		case Running:
			printer.Fprintf("%s %s %s\n", color.CyanString(t.config.RunningStyle), task.Title, elapsedText)
		case Warning:
			printer.Fprintf("%s %s  %s\n", color.YellowString(t.config.WarningStyle), task.Title, elapsedText)
		case Error:
			printer.Fprintf("%s %s %s %s\n", color.RedString(t.config.ErrorStyle), task.Title, elapsedText, color.RedString("(%s)", errorDescription))
		case Success:
			printer.Fprintf("%s %s  %s\n", color.GreenString(t.config.SuccessStyle), task.Title, elapsedText)
		case Skipped:
			printer.Fprintf("%s %s %s\n", color.HiBlackString(t.config.SkippedStyle), task.Title, color.RedString("(%s)", errorDescription))
		}
	}

	printer.Fprintln()

	return nil
}

// runSyncTasks executes all synchronous tasks in order after async tasks are completed.
func (t *TaskList) runSyncTasks() {
	t.syncMutex.Lock()
	defer t.syncMutex.Unlock()

	for _, task := range t.syncTasks {
		task.startTime = Ptr(time.Now())
		task.State = Running

		state, err := task.Action()
		if err != nil {
			t.errorMuxtex.Lock()
			t.errors = append(t.errors, err)
			t.errorMuxtex.Unlock()
		}

		task.endTime = Ptr(time.Now())
		task.Error = err
		task.State = state

		atomic.AddInt32(&t.completed, 1)
	}
}

// addAsyncTask adds an asynchronous task and starts its execution in a goroutine.
func (t *TaskList) addAsyncTask(task *Task) {
	t.waitGroup.Add(1)
	go func() {
		defer t.waitGroup.Done()

		// Acquire a slot in the semaphore
		t.asyncSemaphore <- struct{}{}
		defer func() { <-t.asyncSemaphore }()

		task.startTime = Ptr(time.Now())
		task.State = Running

		state, err := task.Action()
		if err != nil {
			t.errorMuxtex.Lock()
			t.errors = append(t.errors, err)
			t.errorMuxtex.Unlock()
		}

		task.endTime = Ptr(time.Now())
		task.Error = err
		task.State = state

		atomic.AddInt32(&t.completed, 1)
	}()
}

// addSyncTask queues a synchronous task for execution after async completion.
func (t *TaskList) addSyncTask(task *Task) {
	t.syncMutex.Lock()
	defer t.syncMutex.Unlock()

	t.syncTasks = append(t.syncTasks, task)
}

// DurationAsText provides a slightly nicer string representation of a duration
// when compared to default formatting in go, by spelling out the words hour,
// minute and second and providing some spacing and eliding the fractional component
// of the seconds part.
func durationAsText(d time.Duration) string {
	if d.Seconds() < 1.0 {
		return "less than a second"
	}

	var builder strings.Builder

	if (d / time.Hour) > 0 {
		writePart(&builder, fmt.Sprintf("%d", d/time.Hour), "hour")
		d = d - ((d / time.Hour) * time.Hour)
	}

	if (d / time.Minute) > 0 {
		writePart(&builder, fmt.Sprintf("%d", d/time.Minute), "minute")
		d = d - ((d / time.Minute) * time.Minute)
	}

	if (d / time.Second) > 0 {
		writePart(&builder, fmt.Sprintf("%d", d/time.Second), "second")
	}

	return builder.String()
}

// writePart writes the string [part] followed by [unit] into [builder], unless
// part is empty or the string "0". If part is "1", the [unit] string is suffixed
// with s. If builder is non empty, the written string is preceded by a space.
func writePart(builder *strings.Builder, part string, unit string) {
	if part != "" && part != "0" {
		if builder.Len() > 0 {
			builder.WriteByte(' ')
		}

		builder.WriteString(part)
		builder.WriteByte(' ')
		builder.WriteString(unit)
		if part != "1" {
			builder.WriteByte('s')
		}
	}
}
