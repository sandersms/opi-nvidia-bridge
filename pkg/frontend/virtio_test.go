// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022 Dell Inc, or its subsidiaries.
// Copyright (c) 2022 NVIDIA CORPORATION & AFFILIATES. All rights reserved.

// Package frontend implememnts the FrontEnd APIs (host facing) of the storage Server
package frontend

import (
	"bytes"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	pc "github.com/opiproject/opi-api/common/v1/gen/go"
	pb "github.com/opiproject/opi-api/storage/v1alpha1/gen/go"
)

func TestFrontEnd_CreateVirtioBlk(t *testing.T) {
	virtioBlk := &pb.VirtioBlk{
		Id:       &pc.ObjectKey{Value: "virtio-blk-42"},
		PcieId:   &pb.PciEndpoint{PhysicalFunction: 42},
		VolumeId: &pc.ObjectKey{Value: "Malloc42"},
		MaxIoQps: 1,
	}
	tests := map[string]struct {
		in          *pb.VirtioBlk
		out         *pb.VirtioBlk
		spdk        []string
		expectedErr error
	}{
		"valid virtio-blk creation": {
			in:          virtioBlk,
			out:         virtioBlk,
			spdk:        []string{`{"id":%d,"error":{"code":0,"message":""},"result":"VblkEmu0pf0"}`},
			expectedErr: status.Error(codes.OK, ""),
		},
		"spdk virtio-blk creation error": {
			in:          virtioBlk,
			out:         nil,
			spdk:        []string{`{"id":%d,"error":{"code":1,"message":"some internal error"},"result":"VblkEmu0pf0"}`},
			expectedErr: errFailedSpdkCall,
		},
		"spdk virtio-blk creation returned false response with no error": {
			in:          virtioBlk,
			out:         nil,
			spdk:        []string{`{"id":%d,"error":{"code":0,"message":""},"result":""}`},
			expectedErr: errUnexpectedSpdkCallResult,
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			testEnv := createTestEnvironment(true, test.spdk)
			defer testEnv.Close()

			request := &pb.CreateVirtioBlkRequest{VirtioBlk: test.in}
			response, err := testEnv.client.CreateVirtioBlk(testEnv.ctx, request)
			if response != nil {
				wantOut, _ := proto.Marshal(test.out)
				gotOut, _ := proto.Marshal(response)

				if !bytes.Equal(wantOut, gotOut) {
					t.Error("response: expected", test.out, "received", response)
				}
			} else if test.out != nil {
				t.Error("response: expected", test.out, "received nil")
			}

			if err != nil {
				if !strings.Contains(err.Error(), test.expectedErr.Error()) {
					t.Error("expected err contains", test.expectedErr, "received", err)
				}
			} else {
				if test.expectedErr != nil {
					t.Error("expected err contains", test.expectedErr, "received nil")
				}
			}
		})
	}
}
