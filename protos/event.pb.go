// Code generated by protoc-gen-go. DO NOT EDIT.
// source: protos/event.proto

package protos

import (
	context "context"
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type SubscribeType int32

const (
	// 区块事件，payload为BlockFilter
	SubscribeType_BLOCK SubscribeType = 0
)

var SubscribeType_name = map[int32]string{
	0: "BLOCK",
}

var SubscribeType_value = map[string]int32{
	"BLOCK": 0,
}

func (x SubscribeType) String() string {
	return proto.EnumName(SubscribeType_name, int32(x))
}

func (SubscribeType) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_bec55cd27928da5d, []int{0}
}

type SubscribeRequest struct {
	Type                 SubscribeType `protobuf:"varint,1,opt,name=type,proto3,enum=protos.SubscribeType" json:"type,omitempty"`
	Filter               []byte        `protobuf:"bytes,2,opt,name=filter,proto3" json:"filter,omitempty"`
	XXX_NoUnkeyedLiteral struct{}      `json:"-"`
	XXX_unrecognized     []byte        `json:"-"`
	XXX_sizecache        int32         `json:"-"`
}

func (m *SubscribeRequest) Reset()         { *m = SubscribeRequest{} }
func (m *SubscribeRequest) String() string { return proto.CompactTextString(m) }
func (*SubscribeRequest) ProtoMessage()    {}
func (*SubscribeRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_bec55cd27928da5d, []int{0}
}

func (m *SubscribeRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SubscribeRequest.Unmarshal(m, b)
}
func (m *SubscribeRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SubscribeRequest.Marshal(b, m, deterministic)
}
func (m *SubscribeRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SubscribeRequest.Merge(m, src)
}
func (m *SubscribeRequest) XXX_Size() int {
	return xxx_messageInfo_SubscribeRequest.Size(m)
}
func (m *SubscribeRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_SubscribeRequest.DiscardUnknown(m)
}

var xxx_messageInfo_SubscribeRequest proto.InternalMessageInfo

func (m *SubscribeRequest) GetType() SubscribeType {
	if m != nil {
		return m.Type
	}
	return SubscribeType_BLOCK
}

func (m *SubscribeRequest) GetFilter() []byte {
	if m != nil {
		return m.Filter
	}
	return nil
}

type Event struct {
	Payload              []byte   `protobuf:"bytes,1,opt,name=payload,proto3" json:"payload,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Event) Reset()         { *m = Event{} }
func (m *Event) String() string { return proto.CompactTextString(m) }
func (*Event) ProtoMessage()    {}
func (*Event) Descriptor() ([]byte, []int) {
	return fileDescriptor_bec55cd27928da5d, []int{1}
}

func (m *Event) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Event.Unmarshal(m, b)
}
func (m *Event) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Event.Marshal(b, m, deterministic)
}
func (m *Event) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Event.Merge(m, src)
}
func (m *Event) XXX_Size() int {
	return xxx_messageInfo_Event.Size(m)
}
func (m *Event) XXX_DiscardUnknown() {
	xxx_messageInfo_Event.DiscardUnknown(m)
}

var xxx_messageInfo_Event proto.InternalMessageInfo

func (m *Event) GetPayload() []byte {
	if m != nil {
		return m.Payload
	}
	return nil
}

type BlockRange struct {
	Start                string   `protobuf:"bytes,1,opt,name=start,proto3" json:"start,omitempty"`
	End                  string   `protobuf:"bytes,2,opt,name=end,proto3" json:"end,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *BlockRange) Reset()         { *m = BlockRange{} }
func (m *BlockRange) String() string { return proto.CompactTextString(m) }
func (*BlockRange) ProtoMessage()    {}
func (*BlockRange) Descriptor() ([]byte, []int) {
	return fileDescriptor_bec55cd27928da5d, []int{2}
}

func (m *BlockRange) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_BlockRange.Unmarshal(m, b)
}
func (m *BlockRange) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_BlockRange.Marshal(b, m, deterministic)
}
func (m *BlockRange) XXX_Merge(src proto.Message) {
	xxx_messageInfo_BlockRange.Merge(m, src)
}
func (m *BlockRange) XXX_Size() int {
	return xxx_messageInfo_BlockRange.Size(m)
}
func (m *BlockRange) XXX_DiscardUnknown() {
	xxx_messageInfo_BlockRange.DiscardUnknown(m)
}

var xxx_messageInfo_BlockRange proto.InternalMessageInfo

func (m *BlockRange) GetStart() string {
	if m != nil {
		return m.Start
	}
	return ""
}

func (m *BlockRange) GetEnd() string {
	if m != nil {
		return m.End
	}
	return ""
}

type BlockFilter struct {
	Bcname               string      `protobuf:"bytes,1,opt,name=bcname,proto3" json:"bcname,omitempty"`
	Range                *BlockRange `protobuf:"bytes,2,opt,name=range,proto3" json:"range,omitempty"`
	ExcludeTx            bool        `protobuf:"varint,3,opt,name=exclude_tx,json=excludeTx,proto3" json:"exclude_tx,omitempty"`
	ExcludeTxEvent       bool        `protobuf:"varint,4,opt,name=exclude_tx_event,json=excludeTxEvent,proto3" json:"exclude_tx_event,omitempty"`
	Contract             string      `protobuf:"bytes,10,opt,name=contract,proto3" json:"contract,omitempty"`
	EventName            string      `protobuf:"bytes,11,opt,name=event_name,json=eventName,proto3" json:"event_name,omitempty"`
	Initiator            string      `protobuf:"bytes,12,opt,name=initiator,proto3" json:"initiator,omitempty"`
	AuthRequire          string      `protobuf:"bytes,13,opt,name=auth_require,json=authRequire,proto3" json:"auth_require,omitempty"`
	FromAddr             string      `protobuf:"bytes,14,opt,name=from_addr,json=fromAddr,proto3" json:"from_addr,omitempty"`
	ToAddr               string      `protobuf:"bytes,15,opt,name=to_addr,json=toAddr,proto3" json:"to_addr,omitempty"`
	XXX_NoUnkeyedLiteral struct{}    `json:"-"`
	XXX_unrecognized     []byte      `json:"-"`
	XXX_sizecache        int32       `json:"-"`
}

func (m *BlockFilter) Reset()         { *m = BlockFilter{} }
func (m *BlockFilter) String() string { return proto.CompactTextString(m) }
func (*BlockFilter) ProtoMessage()    {}
func (*BlockFilter) Descriptor() ([]byte, []int) {
	return fileDescriptor_bec55cd27928da5d, []int{3}
}

func (m *BlockFilter) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_BlockFilter.Unmarshal(m, b)
}
func (m *BlockFilter) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_BlockFilter.Marshal(b, m, deterministic)
}
func (m *BlockFilter) XXX_Merge(src proto.Message) {
	xxx_messageInfo_BlockFilter.Merge(m, src)
}
func (m *BlockFilter) XXX_Size() int {
	return xxx_messageInfo_BlockFilter.Size(m)
}
func (m *BlockFilter) XXX_DiscardUnknown() {
	xxx_messageInfo_BlockFilter.DiscardUnknown(m)
}

var xxx_messageInfo_BlockFilter proto.InternalMessageInfo

func (m *BlockFilter) GetBcname() string {
	if m != nil {
		return m.Bcname
	}
	return ""
}

func (m *BlockFilter) GetRange() *BlockRange {
	if m != nil {
		return m.Range
	}
	return nil
}

func (m *BlockFilter) GetExcludeTx() bool {
	if m != nil {
		return m.ExcludeTx
	}
	return false
}

func (m *BlockFilter) GetExcludeTxEvent() bool {
	if m != nil {
		return m.ExcludeTxEvent
	}
	return false
}

func (m *BlockFilter) GetContract() string {
	if m != nil {
		return m.Contract
	}
	return ""
}

func (m *BlockFilter) GetEventName() string {
	if m != nil {
		return m.EventName
	}
	return ""
}

func (m *BlockFilter) GetInitiator() string {
	if m != nil {
		return m.Initiator
	}
	return ""
}

func (m *BlockFilter) GetAuthRequire() string {
	if m != nil {
		return m.AuthRequire
	}
	return ""
}

func (m *BlockFilter) GetFromAddr() string {
	if m != nil {
		return m.FromAddr
	}
	return ""
}

func (m *BlockFilter) GetToAddr() string {
	if m != nil {
		return m.ToAddr
	}
	return ""
}

type FilteredBlock struct {
	Bcname               string                 `protobuf:"bytes,1,opt,name=bcname,proto3" json:"bcname,omitempty"`
	Blockid              string                 `protobuf:"bytes,2,opt,name=blockid,proto3" json:"blockid,omitempty"`
	BlockHeight          int64                  `protobuf:"varint,3,opt,name=block_height,json=blockHeight,proto3" json:"block_height,omitempty"`
	Txs                  []*FilteredTransaction `protobuf:"bytes,4,rep,name=txs,proto3" json:"txs,omitempty"`
	XXX_NoUnkeyedLiteral struct{}               `json:"-"`
	XXX_unrecognized     []byte                 `json:"-"`
	XXX_sizecache        int32                  `json:"-"`
}

func (m *FilteredBlock) Reset()         { *m = FilteredBlock{} }
func (m *FilteredBlock) String() string { return proto.CompactTextString(m) }
func (*FilteredBlock) ProtoMessage()    {}
func (*FilteredBlock) Descriptor() ([]byte, []int) {
	return fileDescriptor_bec55cd27928da5d, []int{4}
}

func (m *FilteredBlock) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_FilteredBlock.Unmarshal(m, b)
}
func (m *FilteredBlock) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_FilteredBlock.Marshal(b, m, deterministic)
}
func (m *FilteredBlock) XXX_Merge(src proto.Message) {
	xxx_messageInfo_FilteredBlock.Merge(m, src)
}
func (m *FilteredBlock) XXX_Size() int {
	return xxx_messageInfo_FilteredBlock.Size(m)
}
func (m *FilteredBlock) XXX_DiscardUnknown() {
	xxx_messageInfo_FilteredBlock.DiscardUnknown(m)
}

var xxx_messageInfo_FilteredBlock proto.InternalMessageInfo

func (m *FilteredBlock) GetBcname() string {
	if m != nil {
		return m.Bcname
	}
	return ""
}

func (m *FilteredBlock) GetBlockid() string {
	if m != nil {
		return m.Blockid
	}
	return ""
}

func (m *FilteredBlock) GetBlockHeight() int64 {
	if m != nil {
		return m.BlockHeight
	}
	return 0
}

func (m *FilteredBlock) GetTxs() []*FilteredTransaction {
	if m != nil {
		return m.Txs
	}
	return nil
}

type FilteredTransaction struct {
	Txid                 string           `protobuf:"bytes,1,opt,name=txid,proto3" json:"txid,omitempty"`
	Events               []*ContractEvent `protobuf:"bytes,2,rep,name=events,proto3" json:"events,omitempty"`
	XXX_NoUnkeyedLiteral struct{}         `json:"-"`
	XXX_unrecognized     []byte           `json:"-"`
	XXX_sizecache        int32            `json:"-"`
}

func (m *FilteredTransaction) Reset()         { *m = FilteredTransaction{} }
func (m *FilteredTransaction) String() string { return proto.CompactTextString(m) }
func (*FilteredTransaction) ProtoMessage()    {}
func (*FilteredTransaction) Descriptor() ([]byte, []int) {
	return fileDescriptor_bec55cd27928da5d, []int{5}
}

func (m *FilteredTransaction) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_FilteredTransaction.Unmarshal(m, b)
}
func (m *FilteredTransaction) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_FilteredTransaction.Marshal(b, m, deterministic)
}
func (m *FilteredTransaction) XXX_Merge(src proto.Message) {
	xxx_messageInfo_FilteredTransaction.Merge(m, src)
}
func (m *FilteredTransaction) XXX_Size() int {
	return xxx_messageInfo_FilteredTransaction.Size(m)
}
func (m *FilteredTransaction) XXX_DiscardUnknown() {
	xxx_messageInfo_FilteredTransaction.DiscardUnknown(m)
}

var xxx_messageInfo_FilteredTransaction proto.InternalMessageInfo

func (m *FilteredTransaction) GetTxid() string {
	if m != nil {
		return m.Txid
	}
	return ""
}

func (m *FilteredTransaction) GetEvents() []*ContractEvent {
	if m != nil {
		return m.Events
	}
	return nil
}

func init() {
	proto.RegisterEnum("protos.SubscribeType", SubscribeType_name, SubscribeType_value)
	proto.RegisterType((*SubscribeRequest)(nil), "protos.SubscribeRequest")
	proto.RegisterType((*Event)(nil), "protos.Event")
	proto.RegisterType((*BlockRange)(nil), "protos.BlockRange")
	proto.RegisterType((*BlockFilter)(nil), "protos.BlockFilter")
	proto.RegisterType((*FilteredBlock)(nil), "protos.FilteredBlock")
	proto.RegisterType((*FilteredTransaction)(nil), "protos.FilteredTransaction")
}

func init() { proto.RegisterFile("protos/event.proto", fileDescriptor_bec55cd27928da5d) }

var fileDescriptor_bec55cd27928da5d = []byte{
	// 536 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x74, 0x93, 0x6d, 0x6f, 0xd3, 0x30,
	0x10, 0xc7, 0xc9, 0xda, 0x6e, 0xcb, 0xa5, 0x2d, 0x95, 0x79, 0xb2, 0x3a, 0x10, 0x5d, 0x5e, 0xa0,
	0x80, 0xb4, 0x16, 0x15, 0xc4, 0x7b, 0x3a, 0x31, 0x21, 0x81, 0x40, 0xf2, 0x8a, 0x84, 0x78, 0x13,
	0x39, 0x89, 0xd7, 0x5a, 0xb4, 0x71, 0xe7, 0x38, 0x53, 0xfa, 0x39, 0xf8, 0x56, 0x7c, 0x2a, 0xe4,
	0x73, 0x92, 0x89, 0xa7, 0x77, 0xbe, 0xff, 0xfd, 0x7c, 0x77, 0xf9, 0x3b, 0x07, 0x64, 0xa7, 0x95,
	0x51, 0xc5, 0x4c, 0xdc, 0x88, 0xdc, 0x4c, 0x31, 0x20, 0x87, 0x4e, 0x1b, 0x3f, 0xad, 0xca, 0x9d,
	0xd0, 0xa9, 0xd2, 0x62, 0x56, 0x53, 0xa9, 0xca, 0x8d, 0xe6, 0x69, 0x0d, 0x86, 0x5f, 0x60, 0x74,
	0x59, 0x26, 0x45, 0xaa, 0x65, 0x22, 0x98, 0xb8, 0x2e, 0x45, 0x61, 0xc8, 0x73, 0xe8, 0x9a, 0xfd,
	0x4e, 0x50, 0x6f, 0xe2, 0x45, 0xc3, 0xf9, 0x03, 0x47, 0x16, 0xd3, 0x96, 0x5b, 0xee, 0x77, 0x82,
	0x21, 0x42, 0x1e, 0xc2, 0xe1, 0x95, 0xdc, 0x18, 0xa1, 0xe9, 0xc1, 0xc4, 0x8b, 0xfa, 0xac, 0x8e,
	0xc2, 0x53, 0xe8, 0xbd, 0xb3, 0xe3, 0x10, 0x0a, 0x47, 0x3b, 0xbe, 0xdf, 0x28, 0x9e, 0x61, 0xb9,
	0x3e, 0x6b, 0xc2, 0xf0, 0x35, 0xc0, 0x62, 0xa3, 0xd2, 0xef, 0x8c, 0xe7, 0x2b, 0x41, 0xee, 0x43,
	0xaf, 0x30, 0x5c, 0x1b, 0xa4, 0x7c, 0xe6, 0x02, 0x32, 0x82, 0x8e, 0xc8, 0x33, 0xac, 0xed, 0x33,
	0x7b, 0x0c, 0x7f, 0x1e, 0x40, 0x80, 0xd7, 0x2e, 0xb0, 0x91, 0x1d, 0x20, 0x49, 0x73, 0xbe, 0x15,
	0xf5, 0xc5, 0x3a, 0x22, 0x11, 0xf4, 0xb4, 0x2d, 0x8c, 0x77, 0x83, 0x39, 0x69, 0x3e, 0xe2, 0xb6,
	0x25, 0x73, 0x00, 0x79, 0x02, 0x20, 0xaa, 0x74, 0x53, 0x66, 0x22, 0x36, 0x15, 0xed, 0x4c, 0xbc,
	0xe8, 0x98, 0xf9, 0xb5, 0xb2, 0xac, 0x48, 0x04, 0xa3, 0xdb, 0x74, 0x8c, 0x1e, 0xd3, 0x2e, 0x42,
	0xc3, 0x16, 0x72, 0x9f, 0x3a, 0x86, 0xe3, 0xc6, 0x5c, 0x0a, 0x38, 0x4c, 0x1b, 0x63, 0x13, 0x0b,
	0xc5, 0x38, 0x6a, 0x80, 0x59, 0x1f, 0x95, 0x4f, 0x76, 0xda, 0xc7, 0xe0, 0xcb, 0x5c, 0x1a, 0xc9,
	0x8d, 0xd2, 0xb4, 0xef, 0xb2, 0xad, 0x40, 0x4e, 0xa1, 0xcf, 0x4b, 0xb3, 0x8e, 0xb5, 0xb8, 0x2e,
	0xa5, 0x16, 0x74, 0x80, 0x40, 0x60, 0x35, 0xe6, 0x24, 0x72, 0x02, 0xfe, 0x95, 0x56, 0xdb, 0x98,
	0x67, 0x99, 0xa6, 0x43, 0xd7, 0xdc, 0x0a, 0x6f, 0xb3, 0x4c, 0x93, 0x47, 0x70, 0x64, 0x94, 0x4b,
	0xdd, 0x75, 0x26, 0x19, 0x65, 0x13, 0xe1, 0x0f, 0x0f, 0x06, 0xce, 0x47, 0x91, 0xa1, 0x31, 0xff,
	0xb5, 0x93, 0xc2, 0x51, 0x62, 0x01, 0xd9, 0x3c, 0x46, 0x13, 0xda, 0xe1, 0xf0, 0x18, 0xaf, 0x85,
	0x5c, 0xad, 0x0d, 0x1a, 0xd8, 0x61, 0x01, 0x6a, 0xef, 0x51, 0x22, 0x67, 0xd0, 0x31, 0x55, 0x41,
	0xbb, 0x93, 0x4e, 0x14, 0xcc, 0x4f, 0x9a, 0x97, 0x68, 0x1a, 0x2f, 0x35, 0xcf, 0x0b, 0x9e, 0x1a,
	0xa9, 0x72, 0x66, 0xb9, 0xf0, 0x2b, 0xdc, 0xfb, 0x47, 0x8e, 0x10, 0xe8, 0x9a, 0x4a, 0x66, 0xf5,
	0x60, 0x78, 0x26, 0x67, 0x70, 0x88, 0x26, 0x16, 0xf4, 0x00, 0x8b, 0xb7, 0xff, 0xea, 0x79, 0x6d,
	0x3c, 0xbe, 0x0c, 0xab, 0xa1, 0x17, 0x63, 0x18, 0xfc, 0xf6, 0x13, 0x13, 0x1f, 0x7a, 0x8b, 0x8f,
	0x9f, 0xcf, 0x3f, 0x8c, 0xee, 0xcc, 0x2f, 0xa0, 0x8f, 0xf0, 0xa5, 0xd0, 0x37, 0x32, 0x15, 0xe4,
	0x0d, 0xf8, 0x2d, 0x4b, 0xe8, 0x5f, 0x3b, 0x50, 0xef, 0xca, 0x78, 0xd0, 0x64, 0xf0, 0xf2, 0x4b,
	0x6f, 0x11, 0x7d, 0x7b, 0xb6, 0x92, 0x66, 0x5d, 0x26, 0xd3, 0x54, 0x6d, 0x67, 0x6e, 0xfd, 0xd6,
	0x5c, 0xe6, 0xb3, 0x3f, 0x37, 0x31, 0x71, 0x3b, 0xfa, 0xea, 0x57, 0x00, 0x00, 0x00, 0xff, 0xff,
	0xe7, 0xdd, 0xc8, 0x91, 0xc0, 0x03, 0x00, 0x00,
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConnInterface

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion6

// EventServiceClient is the client API for EventService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type EventServiceClient interface {
	Subscribe(ctx context.Context, in *SubscribeRequest, opts ...grpc.CallOption) (EventService_SubscribeClient, error)
}

type eventServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewEventServiceClient(cc grpc.ClientConnInterface) EventServiceClient {
	return &eventServiceClient{cc}
}

func (c *eventServiceClient) Subscribe(ctx context.Context, in *SubscribeRequest, opts ...grpc.CallOption) (EventService_SubscribeClient, error) {
	stream, err := c.cc.NewStream(ctx, &_EventService_serviceDesc.Streams[0], "/protos.EventService/Subscribe", opts...)
	if err != nil {
		return nil, err
	}
	x := &eventServiceSubscribeClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type EventService_SubscribeClient interface {
	Recv() (*Event, error)
	grpc.ClientStream
}

type eventServiceSubscribeClient struct {
	grpc.ClientStream
}

func (x *eventServiceSubscribeClient) Recv() (*Event, error) {
	m := new(Event)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// EventServiceServer is the server API for EventService service.
type EventServiceServer interface {
	Subscribe(*SubscribeRequest, EventService_SubscribeServer) error
}

// UnimplementedEventServiceServer can be embedded to have forward compatible implementations.
type UnimplementedEventServiceServer struct {
}

func (*UnimplementedEventServiceServer) Subscribe(req *SubscribeRequest, srv EventService_SubscribeServer) error {
	return status.Errorf(codes.Unimplemented, "method Subscribe not implemented")
}

func RegisterEventServiceServer(s *grpc.Server, srv EventServiceServer) {
	s.RegisterService(&_EventService_serviceDesc, srv)
}

func _EventService_Subscribe_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(SubscribeRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(EventServiceServer).Subscribe(m, &eventServiceSubscribeServer{stream})
}

type EventService_SubscribeServer interface {
	Send(*Event) error
	grpc.ServerStream
}

type eventServiceSubscribeServer struct {
	grpc.ServerStream
}

func (x *eventServiceSubscribeServer) Send(m *Event) error {
	return x.ServerStream.SendMsg(m)
}

var _EventService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "protos.EventService",
	HandlerType: (*EventServiceServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Subscribe",
			Handler:       _EventService_Subscribe_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "protos/event.proto",
}