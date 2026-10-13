//go:build !windows

package desktoplaunch

import "log"

func showStartupError(err error) {
	if err != nil {
		log.Printf("startup error: %v", err)
	}
}
