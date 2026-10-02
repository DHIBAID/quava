package clipboard

import (
	"bytes"
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"libquava/models"

	clip "golang.design/x/clipboard"
)

type PushFunc func(context.Context, models.ClipboardContent) error

type ClipboardService struct {
	LastUpdated  time.Time
	Clipboard    models.ClipboardContent
	pendingMutex sync.Mutex
	lifecycle    sync.Mutex
	push         PushFunc
	cancel       context.CancelFunc
	done         chan struct{}
	started      bool
}

func NewClipboardService(push PushFunc) *ClipboardService {
	return &ClipboardService{
		push: push,
	}
}

func (c *ClipboardService) StartClipboardService(ctx context.Context) error {
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()

	if c.started {
		return errors.New("clipboard: service already running")
	}

	if err := clip.Init(); err != nil {
		return err
	}

	workerCtx, cancel := context.WithCancel(ctx)
	changes := clip.Watch(workerCtx, clip.FmtText)

	c.cancel = cancel
	c.done = make(chan struct{})
	c.started = true

	go func() {
		defer close(c.done)

		for {
			select {
			case <-workerCtx.Done():
				return

			case event, ok := <-changes:
				if !ok {
					return
				}

				content := models.ClipboardContent{
					ContentType: models.Text,
					Content:     bytes.Clone(event.Bytes),
				}

				if err := c.PushClipboardContent(workerCtx, content); err != nil {
					if workerCtx.Err() == nil {
						log.Printf("clipboard: sync failed: %v", err)
					}
				}
			}
		}
	}()

	return nil
}

func (c *ClipboardService) PushClipboardContent(ctx context.Context, content models.ClipboardContent) error {
	c.pendingMutex.Lock()

	if c.Clipboard.ContentType == content.ContentType &&
		bytes.Equal(c.Clipboard.Content, content.Content) {
		c.pendingMutex.Unlock()
		return nil
	}

	c.Clipboard = models.ClipboardContent{
		ContentType: content.ContentType,
		Content:     bytes.Clone(content.Content),
	}
	c.LastUpdated = time.Now()

	c.pendingMutex.Unlock()

	if c.push == nil {
		return nil
	}

	return c.push(ctx, content)
}

func (c *ClipboardService) StopClipboardService() {
	c.lifecycle.Lock()

	if !c.started {
		c.lifecycle.Unlock()
		return
	}

	cancel := c.cancel
	done := c.done

	c.started = false
	c.cancel = nil
	c.done = nil

	c.lifecycle.Unlock()

	cancel()
	<-done
}
