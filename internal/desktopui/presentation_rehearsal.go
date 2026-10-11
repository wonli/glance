package desktopui

import (
	"fmt"
	"time"
)

func (v *presentationViewState) rehearsalRunning() bool {
	return v != nil && !v.rehearsalStartedAt.IsZero()
}

func (v *presentationViewState) rehearsalDuration(now time.Time) time.Duration {
	if v == nil {
		return 0
	}
	duration := v.rehearsalElapsed
	if v.rehearsalRunning() {
		duration += now.Sub(v.rehearsalStartedAt)
	}
	if duration < 0 {
		return 0
	}
	return duration
}

func (a *App) startPresentationRehearsal(v *presentationViewState) {
	if v == nil {
		return
	}
	if v.rehearsalRunning() {
		return
	}
	v.rehearsalStartedAt = time.Now()
	stop := make(chan struct{})
	v.rehearsalTickStop = stop
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if window != nil {
					window.Invalidate()
				}
			case <-stop:
				return
			case <-shutdown:
				return
			}
		}
	})
}

func stopPresentationRehearsal(v *presentationViewState, now time.Time) {
	if v == nil || !v.rehearsalRunning() {
		return
	}
	v.rehearsalElapsed += now.Sub(v.rehearsalStartedAt)
	v.rehearsalStartedAt = time.Time{}
	if v.rehearsalTickStop != nil {
		close(v.rehearsalTickStop)
		v.rehearsalTickStop = nil
	}
}

func resetPresentationRehearsal(v *presentationViewState, now time.Time) {
	stopPresentationRehearsal(v, now)
	if v != nil {
		v.rehearsalElapsed = 0
	}
}

func formatPresentationRehearsalDuration(duration time.Duration) string {
	seconds := int64(duration / time.Second)
	hours := seconds / 3600
	minutes := seconds % 3600 / 60
	seconds %= 60
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}
