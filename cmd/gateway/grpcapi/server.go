package grpcapi
)


var gvr = schema.GroupVersionResource{
Group: codespaceapiv1alpha1.GroupVersion.Group,
Version: codespaceapiv1alpha1.GroupVersion.Version,
Resource: "sessions",
}


type Server struct{ dyn dynamic.Interface }


func New(d dynamic.Interface) *Server { return &Server{dyn: d} }


func (s *Server) List(ctx context.Context, in *codespacev1.Namespace) (*codespacev1.SessionList, error) {
ns := in.GetName()
if ns == "" { ns = "default" }
list, err := s.dyn.Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{})
if err != nil { return nil, err }
out := &codespacev1.SessionList{}
for _, it := range list.Items {
b, _ := json.Marshal(it.Object)
st := &structpb.Struct{}; _ = st.UnmarshalJSON(b)
out.Items = append(out.Items, st)
}
return out, nil
}


func (s *Server) Get(ctx context.Context, in *codespacev1.SessionRef) (*codespacev1.SessionObject, error) {
obj, err := s.dyn.Resource(gvr).Namespace(in.Namespace).Get(ctx, in.Name, metav1.GetOptions{})
if err != nil { return nil, err }
b, _ := json.Marshal(obj.Object)
st := &structpb.Struct{}; _ = st.UnmarshalJSON(b)
return &codespacev1.SessionObject{Object: st}, nil
}


func (s *Server) Create(ctx context.Context, in *codespacev1.SessionObject) (*codespacev1.SessionObject, error) {
m := map[string]any{}
b, _ := in.Object.MarshalJSON()
_ = json.Unmarshal(b, &m)
u := &unstructured.Unstructured{Object: m}
ns := u.GetNamespace(); if ns == "" { ns = "default" }
out, err := s.dyn.Resource(gvr).Namespace(ns).Create(ctx, u, metav1.CreateOptions{})
if err != nil { return nil, err }
rb, _ := json.Marshal(out.Object)
st := &structpb.Struct{}; _ = st.UnmarshalJSON(rb)
return &codespacev1.SessionObject{Object: st}, nil
}


func (s *Server) Delete(ctx context.Context, in *codespacev1.SessionRef) (*codespacev1.Empty, error) {
if err := s.dyn.Resource(gvr).Namespace(in.Namespace).Delete(ctx, in.Name, metav1.DeleteOptions{}); err != nil { return nil, err }
return &codespacev1.Empty{}, nil
}


func (s *Server) Scale(ctx context.Context, in *codespacev1.ScaleRequest) (*codespacev1.SessionObject, error) {
patch := []byte(fmt.Sprintf(`+{"spec":{"replicas":%d}}`, in.Replicas))
out, err := s.dyn.Resource(gvr).Namespace(in.Namespace).Patch(ctx, in.Name, types.MergePatchType, patch, metav1.PatchOptions{})
if err != nil { return nil, err }
rb, _ := json.Marshal(out.Object)
st := &structpb.Struct{}; _ = st.UnmarshalJSON(rb)
return &codespacev1.SessionObject{Object: st}, nil
}


func (s *Server) Watch(in *codespacev1.Namespace, stream codespacev1.SessionService_WatchServer) error {
ns := in.GetName(); if ns == "" { ns = "default" }
w, err := s.dyn.Resource(gvr).Namespace(ns).Watch(stream.Context(), metav1.ListOptions{Watch: true})
if err != nil { return err }
defer w.Stop()
for ev := range w.ResultChan() {
if ev.Type == watch.Error { continue }
u, ok := ev.Object.(*unstructured.Unstructured); if !ok { continue }
b, _ := json.Marshal(u.Object)
st := &structpb.Struct{}; _ = st.UnmarshalJSON(b)
if err := stream.Send(&codespacev1.WatchEvent{Type: string(ev.Type), Object: st}); err != nil { return err }
}
return nil
}