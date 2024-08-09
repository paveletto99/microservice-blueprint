package middleware

import "net/http"

// Middleware receives a handler and returns another handler.
// The returned handler can do some customized task according to
// the requirement
type Middleware func(http.Handler) http.Handler

// Chain make middlewares together
func Chain(middlewares ...Middleware) Middleware {
	return func(h http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			h = middlewares[i](h)
		}

		return h
	}
}

// // WithMiddlewares apply the middlewares to the handler.
// // The middlewares are executed in the order that they are applied
// func WithMiddlewares(handler http.Handler, middlewares ...Middleware) http.Handler {
// 	return Chain(middlewares...)(handler)
// }

// // New make a middleware from fn which type is func(w http.ResponseWriter, r *http.Request, next http.Handler)
// func New(fn func(http.ResponseWriter, *http.Request, http.Handler), skippers ...Skipper) func(http.Handler) http.Handler {
// 	return func(next http.Handler) http.Handler {
// 		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			for _, skipper := range skippers {
// 				if skipper(r) {
// 					next.ServeHTTP(w, r)
// 					return
// 				}
// 			}

// 			fn(w, r, next)
// 		})
// 	}
// }

// // BeforeRequest make a middleware which will call hook before the next handler
// func BeforeRequest(hook func(*http.Request) error, skippers ...Skipper) func(http.Handler) http.Handler {
// 	return New(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
// 		if err := hook(r); err != nil {
// 			lib_http.SendError(w, err)
// 			return
// 		}

// 		next.ServeHTTP(w, r)
// 	}, skippers...)
// }

// // AfterResponse make a middleware which will call hook after the next handler
// func AfterResponse(hook func(http.ResponseWriter, *http.Request, int) error, skippers ...Skipper) func(http.Handler) http.Handler {
// 	return New(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
// 		res, ok := w.(*lib.ResponseBuffer)
// 		if !ok {
// 			res = lib.NewResponseBuffer(w)
// 			defer res.Flush()
// 		}

// 		next.ServeHTTP(res, r)

// 		if err := hook(res, r, res.StatusCode()); err != nil {
// 			_ = res.Reset()
// 			lib_http.SendError(res, err)
// 		}
// 	}, skippers...)
// }
