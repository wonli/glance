package desktopui

import "sync"

type trayAction uint8

type systemTrayActionQueue struct {
	actions chan trayAction
	mu      sync.Mutex
}

const (
	trayActionShow trayAction = iota + 1
	trayActionQuit
)

func (q *systemTrayActionQueue) send(action trayAction) {
	if q == nil || q.actions == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	switch action {
	case trayActionShow:
		for {
			select {
			case queued := <-q.actions:
				if queued == trayActionQuit {
					q.actions <- queued
					return
				}
			default:
				q.actions <- trayActionShow
				return
			}
		}
	case trayActionQuit:
		for {
			select {
			case <-q.actions:
			default:
				q.actions <- trayActionQuit
				return
			}
		}
	default:
		select {
		case q.actions <- action:
		default:
		}
	}
}
