// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.21.7
// source: gateway.protos

package gatewaypb

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// GatewayClient is the client API for Gateway service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type GatewayClient interface {
	Version(ctx context.Context, in *EmptyRequest, opts ...grpc.CallOption) (*VersionResponse, error)
	RequestGuildMembers(ctx context.Context, in *RequestGuildMembersRequest, opts ...grpc.CallOption) (*EmptyResponse, error)
	RestartShard(ctx context.Context, in *RestartShardRequest, opts ...grpc.CallOption) (*EmptyResponse, error)
	Stats(ctx context.Context, in *EmptyRequest, opts ...grpc.CallOption) (*StatsResponse, error)
}

type gatewayClient struct {
	cc grpc.ClientConnInterface
}

func NewGatewayClient(cc grpc.ClientConnInterface) GatewayClient {
	return &gatewayClient{cc}
}

func (c *gatewayClient) Version(ctx context.Context, in *EmptyRequest, opts ...grpc.CallOption) (*VersionResponse, error) {
	out := new(VersionResponse)
	err := c.cc.Invoke(ctx, "/gateway.Gateway/Version", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gatewayClient) RequestGuildMembers(ctx context.Context, in *RequestGuildMembersRequest, opts ...grpc.CallOption) (*EmptyResponse, error) {
	out := new(EmptyResponse)
	err := c.cc.Invoke(ctx, "/gateway.Gateway/RequestGuildMembers", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gatewayClient) RestartShard(ctx context.Context, in *RestartShardRequest, opts ...grpc.CallOption) (*EmptyResponse, error) {
	out := new(EmptyResponse)
	err := c.cc.Invoke(ctx, "/gateway.Gateway/RestartShard", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gatewayClient) Stats(ctx context.Context, in *EmptyRequest, opts ...grpc.CallOption) (*StatsResponse, error) {
	out := new(StatsResponse)
	err := c.cc.Invoke(ctx, "/gateway.Gateway/Stats", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GatewayServer is the server API for Gateway service.
// All implementations must embed UnimplementedGatewayServer
// for forward compatibility
type GatewayServer interface {
	Version(context.Context, *EmptyRequest) (*VersionResponse, error)
	RequestGuildMembers(context.Context, *RequestGuildMembersRequest) (*EmptyResponse, error)
	RestartShard(context.Context, *RestartShardRequest) (*EmptyResponse, error)
	Stats(context.Context, *EmptyRequest) (*StatsResponse, error)
	mustEmbedUnimplementedGatewayServer()
}

// UnimplementedGatewayServer must be embedded to have forward compatible implementations.
type UnimplementedGatewayServer struct {
}

func (UnimplementedGatewayServer) Version(context.Context, *EmptyRequest) (*VersionResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Version not implemented")
}
func (UnimplementedGatewayServer) RequestGuildMembers(context.Context, *RequestGuildMembersRequest) (*EmptyResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RequestGuildMembers not implemented")
}
func (UnimplementedGatewayServer) RestartShard(context.Context, *RestartShardRequest) (*EmptyResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RestartShard not implemented")
}
func (UnimplementedGatewayServer) Stats(context.Context, *EmptyRequest) (*StatsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Stats not implemented")
}
func (UnimplementedGatewayServer) mustEmbedUnimplementedGatewayServer() {}

// UnsafeGatewayServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to GatewayServer will
// result in compilation errors.
type UnsafeGatewayServer interface {
	mustEmbedUnimplementedGatewayServer()
}

func RegisterGatewayServer(s grpc.ServiceRegistrar, srv GatewayServer) {
	s.RegisterService(&Gateway_ServiceDesc, srv)
}

func _Gateway_Version_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(EmptyRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GatewayServer).Version(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gateway.Gateway/Version",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GatewayServer).Version(ctx, req.(*EmptyRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Gateway_RequestGuildMembers_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RequestGuildMembersRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GatewayServer).RequestGuildMembers(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gateway.Gateway/RequestGuildMembers",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GatewayServer).RequestGuildMembers(ctx, req.(*RequestGuildMembersRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Gateway_RestartShard_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RestartShardRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GatewayServer).RestartShard(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gateway.Gateway/RestartShard",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GatewayServer).RestartShard(ctx, req.(*RestartShardRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Gateway_Stats_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(EmptyRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GatewayServer).Stats(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gateway.Gateway/Stats",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GatewayServer).Stats(ctx, req.(*EmptyRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Gateway_ServiceDesc is the grpc.ServiceDesc for Gateway service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Gateway_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "gateway.Gateway",
	HandlerType: (*GatewayServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Version",
			Handler:    _Gateway_Version_Handler,
		},
		{
			MethodName: "RequestGuildMembers",
			Handler:    _Gateway_RequestGuildMembers_Handler,
		},
		{
			MethodName: "RestartShard",
			Handler:    _Gateway_RestartShard_Handler,
		},
		{
			MethodName: "Stats",
			Handler:    _Gateway_Stats_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "gateway.protos",
}