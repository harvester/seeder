package endpoint

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	"golang.org/x/sync/errgroup"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

type Server struct {
	ctx    context.Context
	client client.Client
	log    logr.Logger
	route  *mux.Router
}

func NewServer(ctx context.Context, client client.Client, log logr.Logger) *Server {
	s := &Server{
		ctx:    ctx,
		client: client,
		log:    log,
	}
	r := mux.NewRouter()
	r.HandleFunc("/disable/{namespace}/{name}", s.disableHardware).Methods("PUT")
	s.route = r
	return s
}

func (s *Server) Start() error {
	srv := &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%d", seederv1alpha1.DefaultEndpointPort),
		WriteTimeout: 5 * time.Second,
		ReadTimeout:  5 * time.Second,
		IdleTimeout:  5 * time.Second,
		Handler:      s.route,
	}

	eg, egctx := errgroup.WithContext(s.ctx)
	eg.Go(func() error {
		err := srv.ListenAndServe()
		if err != http.ErrServerClosed {
			return err
		}
		return nil
	})

	eg.Go(func() error {
		<-egctx.Done()
		return srv.Shutdown(egctx)
	})

	return eg.Wait()
}

func (s *Server) disableHardware(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name, ok := vars["name"]
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		s.log.Error(fmt.Errorf("error: no hw name specified"), "")
		return
	}

	namespace, ok := vars["namespace"]
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		s.log.Error(fmt.Errorf("error: no hw namespace specified"), "")
		return
	}

	hwObj := &tinkv1alpha1.Hardware{}
	if err := s.client.Get(s.ctx, types.NamespacedName{Name: name, Namespace: namespace}, hwObj); err != nil {
		if apierrors.IsNotFound(err) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		s.log.Error(err, "error looking up hw object", name, namespace)
		return
	}

	// disable Netboot AllowPXE on all interfaces in hwObj
	for i, netifs := range hwObj.Spec.Interfaces {
		if *netifs.Netboot.AllowPXE {
			hwObj.Spec.Interfaces[i].Netboot.AllowPXE = &[]bool{false}[0]

		}
	}

	if err := s.client.Update(s.ctx, hwObj); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		s.log.Error(err, "error disabling AllowPXE", hwObj.Name, hwObj.Namespace)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
