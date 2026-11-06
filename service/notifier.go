package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/qw145/hotelbooking/helper"
	"github.com/qw145/hotelbooking/model"
)

type Notifier interface {
	// BookingConfirmed enqueues a confirmation notification. It must not
	// block on the actual send.
	SendConfirmation(ctx context.Context) error
}

type EmailSender struct {
	Booking *model.Booking
	Log     *slog.Logger
}

func (s *EmailSender) SendConfirmation(ctx context.Context) error {
	select {
	case <-time.After(helper.Cfg.EmailSendDelay):
		s.Log.Info("[EMAIL] s.booking confirmation",
			"to", s.Booking.GuestEmail,
			"guest", s.Booking.GuestName,
			"reference", s.Booking.Reference,
			"check_in", s.Booking.CheckIn.Format(time.DateOnly),
			"check_out", s.Booking.CheckOut.Format(time.DateOnly),
			"nights", helper.NightsBetween(s.Booking.CheckIn, s.Booking.CheckOut),
			"guests", s.Booking.Guests,
		)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type Result struct {
	Err error
}

type Worker struct {
	taskChan <-chan Notifier
	results  chan<- Result
	wg       *sync.WaitGroup
}

func NewWorker(taskChan <-chan Notifier, results chan<- Result, wg *sync.WaitGroup) *Worker {
	return &Worker{
		taskChan: taskChan,
		wg:       wg,
		results:  results,
	}
}

func (wr *Worker) Start(ctx context.Context) {
	defer wr.wg.Done()

	for {
		select {
		case task, ok := <-wr.taskChan:
			if !ok {
				return
			}
			err := task.SendConfirmation(ctx)
			wr.results <- Result{Err: err}
		case <-ctx.Done():
			return
		}
	}
}

type Pool struct {
	workers []*Worker

	concurrency int
	Collector   chan Notifier
	Results     chan Result
	Wg          sync.WaitGroup
}

func NewPool(concurrency, jobSize int) *Pool {
	return &Pool{
		concurrency: concurrency,
		Collector:   make(chan Notifier, jobSize),
		Results:     make(chan Result, jobSize),
	}
}

func (p *Pool) Run(ctx context.Context) {
	for i := 1; i <= p.concurrency; i++ {
		worker := NewWorker(p.Collector, p.Results, &p.Wg)
		p.workers = append(p.workers, worker)
		p.Wg.Add(1)
		go worker.Start(ctx)
	}
}
