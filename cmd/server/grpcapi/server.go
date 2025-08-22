package grpcapi

import (
	"context"
	"encoding/json"
	"fmt"

	apiv1 "github.com/codespace-operator/codespace-operator/api/v1"
	pb "github.com/codespace-operator/codespace-operator/pkg/gen/codespace/v1"
	"google.golang.org/protobuf/types/known/structpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

var gvr = schema.GroupVersionResource{
	Group:    apiv1.GroupVersion.Group,
	Version:  apiv1.GroupVersion.Version,
	Resource: "sessions",
}

type Server struct {
	pb.UnimplementedSessionServiceServer // <— embed the stub to satisfy the interface
	dyn dynamic.Interface
}
func New(d dynamic.Interface) *Server { return &Server{dyn: d} }

// List implements pb.SessionServiceServer.
func (s *Server) List(ctx context.Context, in *pb.Namespace) (*pb.SessionList, error) {
	ns := in.GetNamespace()
	if ns == "" {
		ns = "default"
	}
	list, err := s.dyn.Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := &pb.SessionList{}
	for _, it := range list.Items {
		b, _ := json.Marshal(it.Object)
		st := &structpb.Struct{}
		_ = st.UnmarshalJSON(b)
		out.Items = append(out.Items, st)
	}
	return out, nil
}

func (s *Server) Get(ctx context.Context, in *pb.SessionRef) (*pb.SessionObject, error) {
	obj, err := s.dyn.Resource(gvr).Namespace(in.GetNamespace()).Get(ctx, in.GetName(), metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(obj.Object)
	st := &structpb.Struct{}
	_ = st.UnmarshalJSON(b)
	return &pb.SessionObject{Object: st}, nil
}

func (s *Server) Create(ctx context.Context, in *pb.SessionObject) (*pb.SessionObject, error) {
	m := map[string]any{}
	b, _ := in.GetObject().MarshalJSON()
	_ = json.Unmarshal(b, &m)

	u := &unstructured.Unstructured{Object: m}
	ns := u.GetNamespace()
	if ns == "" {
		ns = "default"
	}
	out, err := s.dyn.Resource(gvr).Namespace(ns).Create(ctx, u, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	rb, _ := json.Marshal(out.Object)
	st := &structpb.Struct{}
	_ = st.UnmarshalJSON(rb)
	return &pb.SessionObject{Object: st}, nil
}

func (s *Server) Delete(ctx context.Context, in *pb.SessionRef) (*pb.Empty, error) {
	if err := s.dyn.Resource(gvr).Namespace(in.GetNamespace()).Delete(ctx, in.GetName(), metav1.DeleteOptions{}); err != nil {
		return nil, err
	}
	return &pb.Empty{}, nil
}

func (s *Server) Scale(ctx context.Context, in *pb.ScaleRequest) (*pb.SessionObject, error) {
	patch := []byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, in.GetReplicas()))
	out, err := s.dyn.Resource(gvr).Namespace(in.GetNamespace()).Patch(
		ctx, in.GetName(), types.MergePatchType, patch, metav1.PatchOptions{},
	)
	if err != nil {
		return nil, err
	}
	rb, _ := json.Marshal(out.Object)
	st := &structpb.Struct{}
	_ = st.UnmarshalJSON(rb)
	return &pb.SessionObject{Object: st}, nil
}

func (s *Server) Watch(in *pb.Namespace, stream pb.SessionService_WatchServer) error {
	ns := in.GetNamespace()
	if ns == "" {
		ns = "default"
	}
	w, err := s.dyn.Resource(gvr).Namespace(ns).Watch(stream.Context(), metav1.ListOptions{Watch: true})
	if err != nil {
		return err
	}
	defer w.Stop()

	for ev := range w.ResultChan() {
		if ev.Type == watch.Error {
			continue
		}
		u, ok := ev.Object.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		b, _ := json.Marshal(u.Object)
		st := &structpb.Struct{}
		_ = st.UnmarshalJSON(b)

		if err := stream.Send(&pb.WatchEvent{
			Type:   string(ev.Type),
			Object: st,
		}); err != nil {
			return err
		}
	}
	return nil
}
