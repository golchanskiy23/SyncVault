package errors

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
)

func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC recovered: %v\n%s", err, debug.Stack())
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func SafeGo(fn func()) {
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC in goroutine: %v\n%s", err, debug.Stack())
			}
		}()

		fn()
	}()
}

func SafeExecute(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = New(
				ErrCodeInternal,
				"SafeExecute",
				fmt.Sprintf("panic recovered: %v", r),
			)
		}
	}()

	return fn()
}
