// Package cron provides a cron-like job scheduler.
package cron

import (
	"container/heap"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Job represents a scheduled task in the scheduler.
type Job struct {
	schedule string
	task     func()
	nextRun  int64
	index    int
}

type jobHeap []*Job

func (h jobHeap) Len() int           { return len(h) }
func (h jobHeap) Less(i, j int) bool { return h[i].nextRun < h[j].nextRun }
func (h jobHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *jobHeap) Push(x interface{}) {
	n := len(*h)
	job := x.(*Job)
	job.index = n
	*h = append(*h, job)
}

func (h *jobHeap) Pop() interface{} {
	old := *h
	n := len(old)
	job := old[n-1]
	old[n-1] = nil // avoid memory leak
	job.index = -1 // for safety
	*h = old[0 : n-1]
	return job
}

// Scheduler manages a collection of jobs and ensures they are run at the scheduled times.
type Scheduler struct {
	jobs    jobHeap
	mutex   sync.Mutex
	wg      sync.WaitGroup
	trigger chan struct{}
	stop    chan struct{}
}

// NewScheduler creates and returns a new Scheduler.
// It immediately starts the scheduling goroutine.
func NewScheduler() *Scheduler {
	s := &Scheduler{
		jobs:    make(jobHeap, 0),
		trigger: make(chan struct{}, 1),
		stop:    make(chan struct{}),
	}
	go s.run()
	return s
}

// AddJob adds a new job to the scheduler.
// The schedule should be a cron-like string with five fields: minute, hour, day of month, month, and day of week.
// It returns a pointer to the created Job and an error if the schedule is invalid.
func (s *Scheduler) AddJob(schedule string, task func()) (*Job, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	nextRun, err := calculateNextRun(schedule, time.Now().Unix())
	if err != nil {
		return nil, err
	}

	job := &Job{
		schedule: schedule,
		task:     task,
		nextRun:  nextRun,
	}
	heap.Push(&s.jobs, job)
	s.triggerRun()
	return job, nil
}

// RemoveJob removes a job from the scheduler.
// If the job is not in the scheduler (i.e., has already been removed), this operation is a no-op.
func (s *Scheduler) RemoveJob(job *Job) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if job.index != -1 {
		heap.Remove(&s.jobs, job.index)
		s.triggerRun()
	}
}

// UpdateJob updates the schedule of an existing job.
// It returns an error if the job is not found in the scheduler or if the new schedule is invalid.
func (s *Scheduler) UpdateJob(job *Job, schedule string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if job.index == -1 {
		return errors.New("job not found in scheduler")
	}

	nextRun, err := calculateNextRun(schedule, time.Now().Unix())
	if err != nil {
		return err
	}

	job.schedule = schedule
	job.nextRun = nextRun
	heap.Fix(&s.jobs, job.index)
	s.triggerRun()
	return nil
}

func (s *Scheduler) triggerRun() {
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

func (s *Scheduler) run() {
	timer := time.NewTimer(time.Hour) // Start with a long duration
	defer timer.Stop()

	for {
		s.mutex.Lock()
		if s.jobs.Len() == 0 {
			s.mutex.Unlock()
			select {
			case <-s.trigger:
				continue
			case <-s.stop:
				return
			}
		}

		now := time.Now().Unix()
		nextJob := s.jobs[0]
		if now < nextJob.nextRun {
			// Reset the timer
			sleepDuration := time.Duration(nextJob.nextRun-now) * time.Second
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(sleepDuration)
			s.mutex.Unlock()

			select {
			case <-timer.C:
			case <-s.trigger:
			case <-s.stop:
				return
			}
			continue
		}

		heap.Pop(&s.jobs)
		s.wg.Add(1)
		go func(job *Job) {
			defer s.wg.Done()
			job.task()
		}(nextJob)

		var err error
		nextJob.nextRun, err = calculateNextRun(nextJob.schedule, now)
		if err == nil {
			heap.Push(&s.jobs, nextJob)
		} else {
			fmt.Printf("Error calculating next run for job: %v\n", err)
		}
		s.mutex.Unlock()
	}
}

// Stop terminates the scheduler, preventing any new jobs from running.
// It waits for any currently running jobs to complete before returning.
func (s *Scheduler) Stop() {
	close(s.stop)
	s.wg.Wait()
	s.mutex.Lock()
	s.jobs = nil
	s.mutex.Unlock()
}

func calculateNextRun(schedule string, now int64) (int64, error) {
	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return 0, errors.New("invalid schedule format")
	}

	t := time.Unix(now, 0).UTC()
	next := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC)

	minute, err := parseField(fields[0], 0, 59)
	if err != nil {
		return 0, fmt.Errorf("invalid minute field: %v", err)
	}
	hour, err := parseField(fields[1], 0, 23)
	if err != nil {
		return 0, fmt.Errorf("invalid hour field: %v", err)
	}
	dayOfMonth, err := parseField(fields[2], 1, 31)
	if err != nil {
		return 0, fmt.Errorf("invalid day of month field: %v", err)
	}
	month, err := parseField(fields[3], 1, 12)
	if err != nil {
		return 0, fmt.Errorf("invalid month field: %v", err)
	}
	dayOfWeek, err := parseField(fields[4], 0, 6)
	if err != nil {
		return 0, fmt.Errorf("invalid day of week field: %v", err)
	}

	for i := 0; i < 1000000; i++ { // Prevent infinite loop
		if month[next.Month()] &&
			(dayOfMonth[next.Day()] || dayOfWeek[next.Weekday()]) &&
			hour[next.Hour()] &&
			minute[next.Minute()] {
			return next.Unix(), nil
		}
		next = next.Add(time.Minute)
	}

	return 0, errors.New("could not find next run time within reasonable limit")
}

func parseField(field string, min, max int) ([]bool, error) {
	result := make([]bool, max+1)
	for _, part := range strings.Split(field, ",") {
		if part == "*" {
			for i := min; i <= max; i++ {
				result[i] = true
			}
		} else if strings.Contains(part, "-") {
			ranges := strings.Split(part, "-")
			start, err := strconv.Atoi(ranges[0])
			if err != nil {
				return nil, err
			}
			end, err := strconv.Atoi(ranges[1])
			if err != nil {
				return nil, err
			}
			if start < min || end > max || start > end {
				return nil, errors.New("invalid range")
			}
			for i := start; i <= end; i++ {
				result[i] = true
			}
		} else if strings.Contains(part, "/") {
			parts := strings.Split(part, "/")
			step, err := strconv.Atoi(parts[1])
			if err != nil {
				return nil, err
			}
			if step < 1 {
				return nil, errors.New("invalid step value")
			}
			for i := min; i <= max; i += step {
				result[i] = true
			}
		} else {
			i, err := strconv.Atoi(part)
			if err != nil {
				return nil, err
			}
			if i < min || i > max {
				return nil, errors.New("value out of range")
			}
			result[i] = true
		}
	}
	return result, nil
}
