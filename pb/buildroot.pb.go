// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.34.1
// 	protoc        v4.24.4
// source: buildroot.proto

package pb

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type BuildRoot struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Meta *BuildRootMeta `protobuf:"bytes,10,opt,name=meta,proto3" json:"meta,omitempty"`
	// store path hash as nix32 (base of .narinfo file in nix cache)
	StorePathHash []string `protobuf:"bytes,5,rep,name=store_path_hash,json=storePathHash,proto3" json:"store_path_hash,omitempty"`
	// manifest cache key
	Manifest []string `protobuf:"bytes,6,rep,name=manifest,proto3" json:"manifest,omitempty"`
}

func (x *BuildRoot) Reset() {
	*x = BuildRoot{}
	if protoimpl.UnsafeEnabled {
		mi := &file_buildroot_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BuildRoot) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BuildRoot) ProtoMessage() {}

func (x *BuildRoot) ProtoReflect() protoreflect.Message {
	mi := &file_buildroot_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BuildRoot.ProtoReflect.Descriptor instead.
func (*BuildRoot) Descriptor() ([]byte, []int) {
	return file_buildroot_proto_rawDescGZIP(), []int{0}
}

func (x *BuildRoot) GetMeta() *BuildRootMeta {
	if x != nil {
		return x.Meta
	}
	return nil
}

func (x *BuildRoot) GetStorePathHash() []string {
	if x != nil {
		return x.StorePathHash
	}
	return nil
}

func (x *BuildRoot) GetManifest() []string {
	if x != nil {
		return x.Manifest
	}
	return nil
}

type BuildRootMeta struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	BuildTime  int64  `protobuf:"varint,1,opt,name=build_time,json=buildTime,proto3" json:"build_time,omitempty"`
	NixRelId   string `protobuf:"bytes,2,opt,name=nix_rel_id,json=nixRelId,proto3" json:"nix_rel_id,omitempty"`
	StyxCommit string `protobuf:"bytes,3,opt,name=styx_commit,json=styxCommit,proto3" json:"styx_commit,omitempty"`
}

func (x *BuildRootMeta) Reset() {
	*x = BuildRootMeta{}
	if protoimpl.UnsafeEnabled {
		mi := &file_buildroot_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BuildRootMeta) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BuildRootMeta) ProtoMessage() {}

func (x *BuildRootMeta) ProtoReflect() protoreflect.Message {
	mi := &file_buildroot_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BuildRootMeta.ProtoReflect.Descriptor instead.
func (*BuildRootMeta) Descriptor() ([]byte, []int) {
	return file_buildroot_proto_rawDescGZIP(), []int{1}
}

func (x *BuildRootMeta) GetBuildTime() int64 {
	if x != nil {
		return x.BuildTime
	}
	return 0
}

func (x *BuildRootMeta) GetNixRelId() string {
	if x != nil {
		return x.NixRelId
	}
	return ""
}

func (x *BuildRootMeta) GetStyxCommit() string {
	if x != nil {
		return x.StyxCommit
	}
	return ""
}

var File_buildroot_proto protoreflect.FileDescriptor

var file_buildroot_proto_rawDesc = []byte{
	0x0a, 0x0f, 0x62, 0x75, 0x69, 0x6c, 0x64, 0x72, 0x6f, 0x6f, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x12, 0x02, 0x70, 0x62, 0x22, 0x76, 0x0a, 0x09, 0x42, 0x75, 0x69, 0x6c, 0x64, 0x52, 0x6f,
	0x6f, 0x74, 0x12, 0x25, 0x0a, 0x04, 0x6d, 0x65, 0x74, 0x61, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x11, 0x2e, 0x70, 0x62, 0x2e, 0x42, 0x75, 0x69, 0x6c, 0x64, 0x52, 0x6f, 0x6f, 0x74, 0x4d,
	0x65, 0x74, 0x61, 0x52, 0x04, 0x6d, 0x65, 0x74, 0x61, 0x12, 0x26, 0x0a, 0x0f, 0x73, 0x74, 0x6f,
	0x72, 0x65, 0x5f, 0x70, 0x61, 0x74, 0x68, 0x5f, 0x68, 0x61, 0x73, 0x68, 0x18, 0x05, 0x20, 0x03,
	0x28, 0x09, 0x52, 0x0d, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x50, 0x61, 0x74, 0x68, 0x48, 0x61, 0x73,
	0x68, 0x12, 0x1a, 0x0a, 0x08, 0x6d, 0x61, 0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x18, 0x06, 0x20,
	0x03, 0x28, 0x09, 0x52, 0x08, 0x6d, 0x61, 0x6e, 0x69, 0x66, 0x65, 0x73, 0x74, 0x22, 0x6d, 0x0a,
	0x0d, 0x42, 0x75, 0x69, 0x6c, 0x64, 0x52, 0x6f, 0x6f, 0x74, 0x4d, 0x65, 0x74, 0x61, 0x12, 0x1d,
	0x0a, 0x0a, 0x62, 0x75, 0x69, 0x6c, 0x64, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x03, 0x52, 0x09, 0x62, 0x75, 0x69, 0x6c, 0x64, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x1c, 0x0a,
	0x0a, 0x6e, 0x69, 0x78, 0x5f, 0x72, 0x65, 0x6c, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x08, 0x6e, 0x69, 0x78, 0x52, 0x65, 0x6c, 0x49, 0x64, 0x12, 0x1f, 0x0a, 0x0b, 0x73,
	0x74, 0x79, 0x78, 0x5f, 0x63, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x0a, 0x73, 0x74, 0x79, 0x78, 0x43, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x42, 0x18, 0x5a, 0x16,
	0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x64, 0x6e, 0x72, 0x2f, 0x73,
	0x74, 0x79, 0x78, 0x2f, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_buildroot_proto_rawDescOnce sync.Once
	file_buildroot_proto_rawDescData = file_buildroot_proto_rawDesc
)

func file_buildroot_proto_rawDescGZIP() []byte {
	file_buildroot_proto_rawDescOnce.Do(func() {
		file_buildroot_proto_rawDescData = protoimpl.X.CompressGZIP(file_buildroot_proto_rawDescData)
	})
	return file_buildroot_proto_rawDescData
}

var file_buildroot_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_buildroot_proto_goTypes = []interface{}{
	(*BuildRoot)(nil),     // 0: pb.BuildRoot
	(*BuildRootMeta)(nil), // 1: pb.BuildRootMeta
}
var file_buildroot_proto_depIdxs = []int32{
	1, // 0: pb.BuildRoot.meta:type_name -> pb.BuildRootMeta
	1, // [1:1] is the sub-list for method output_type
	1, // [1:1] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_buildroot_proto_init() }
func file_buildroot_proto_init() {
	if File_buildroot_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_buildroot_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BuildRoot); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_buildroot_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BuildRootMeta); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_buildroot_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_buildroot_proto_goTypes,
		DependencyIndexes: file_buildroot_proto_depIdxs,
		MessageInfos:      file_buildroot_proto_msgTypes,
	}.Build()
	File_buildroot_proto = out.File
	file_buildroot_proto_rawDesc = nil
	file_buildroot_proto_goTypes = nil
	file_buildroot_proto_depIdxs = nil
}