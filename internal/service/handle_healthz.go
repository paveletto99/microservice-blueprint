package service

import (
	"encoding/json"
	"net/http"

	"github.com/sirupsen/logrus"
)

type Healthz struct {
	Status string `json:"status"`
}

func HandleHealthz(log *logrus.Logger) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(writer).Encode(Healthz{Status: "ok"}); err != nil {
			log.Error("failed to serve healthcheck")
			writer.WriteHeader(http.StatusInternalServerError)
		}
	})
}

// example of scyllaDB operator

// func (p *Prober) Healthz(w http.ResponseWriter, req *http.Request) {
// 	ctx, ctxCancel := context.WithTimeout(req.Context(), p.timeout)
// 	defer ctxCancel()

// 	underMaintenance, err := p.isNodeUnderMaintenance()
// 	if err != nil {
// 		w.WriteHeader(http.StatusServiceUnavailable)
// 		klog.ErrorS(err, "healthz probe: can't look up service maintenance label", "Service", p.serviceRef())
// 		return
// 	}

// 	if underMaintenance {
// 		w.WriteHeader(http.StatusOK)
// 		klog.V(2).InfoS("healthz probe: node is under maintenance", "Service", p.serviceRef())
// 		return
// 	}

// 	scyllaClient, err := controllerhelpers.NewScyllaClientForLocalhost()
// 	if err != nil {
// 		klog.ErrorS(err, "healthz probe: can't get scylla client", "Service", p.serviceRef())
// 		w.WriteHeader(http.StatusInternalServerError)
// 		return
// 	}
// 	defer scyllaClient.Close()

// 	// Check if Scylla API is reachable
// 	_, err = scyllaClient.Ping(ctx, localhost)
// 	if err != nil {
// 		klog.ErrorS(err, "healthz probe: can't connect to Scylla API", "Service", p.serviceRef())
// 		w.WriteHeader(http.StatusServiceUnavailable)
// 		return
// 	}

// 	w.WriteHeader(http.StatusOK)
// }