// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (c) 2022 NVIDIA CORPORATION & AFFILIATES. All rights reserved.

// Package frontend implememnts the FrontEnd APIs (host facing) of the storage Server
package frontend

import (
	"context"
	"fmt"
	"log"
	"path"
	"sort"

	pc "github.com/opiproject/opi-api/common/v1/gen/go"
	pb "github.com/opiproject/opi-api/storage/v1alpha1/gen/go"
	"github.com/opiproject/opi-nvidia-bridge/pkg/models"
	"github.com/opiproject/opi-spdk-bridge/pkg/server"

	"github.com/google/uuid"
	"go.einride.tech/aip/fieldbehavior"
	"go.einride.tech/aip/fieldmask"
	"go.einride.tech/aip/resourceid"
	"go.einride.tech/aip/resourcename"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func sortVirtioBlks(virtioBlks []*pb.VirtioBlk) {
	sort.Slice(virtioBlks, func(i int, j int) bool {
		return virtioBlks[i].Name < virtioBlks[j].Name
	})
}

// CreateVirtioBlk creates a Virtio block device
func (s *Server) CreateVirtioBlk(_ context.Context, in *pb.CreateVirtioBlkRequest) (*pb.VirtioBlk, error) {
	log.Printf("CreateVirtioBlk: Received from client: %v", in)
	// see https://google.aip.dev/133#user-specified-ids
	resourceID := resourceid.NewSystemGenerated()
	if in.VirtioBlkId != "" {
		err := resourceid.ValidateUserSettable(in.VirtioBlkId)
		if err != nil {
			log.Printf("error: %v", err)
			return nil, err
		}
		log.Printf("client provided the ID of a resource %v, ignoring the name field %v", in.VirtioBlkId, in.VirtioBlk.Name)
		resourceID = in.VirtioBlkId
	}
	in.VirtioBlk.Name = server.ResourceIDToVolumeName(resourceID)
	// idempotent API when called with same key, should return same object
	controller, ok := s.VirtioCtrls[in.VirtioBlk.Name]
	if ok {
		log.Printf("Already existing NvmeController with id %v", in.VirtioBlk.Name)
		return controller, nil
	}
	// not found, so create a new one
	params := models.NvdaControllerVirtioBlkCreateParams{
		Serial: resourceID,
		Bdev:   in.VirtioBlk.VolumeId.Value,
		PfID:   int(in.VirtioBlk.PcieId.PhysicalFunction),
		// VfID:             int(in.VirtioBlk.PcieId.VirtualFunction),
		NumQueues:        int(in.VirtioBlk.MaxIoQps),
		BdevType:         "spdk",
		EmulationManager: "mlx5_0",
	}
	var result models.NvdaControllerVirtioBlkCreateResult
	err := s.rpc.Call("controller_virtio_blk_create", &params, &result)
	if err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	log.Printf("Received from SPDK: %v", result)
	if result == "" {
		msg := fmt.Sprintf("Could not create virtio-blk: %s", resourceID)
		log.Print(msg)
		return nil, status.Errorf(codes.InvalidArgument, msg)
	}
	response := server.ProtoClone(in.VirtioBlk)
	// response.Status = &pb.NvmeControllerStatus{Active: true}
	s.VirtioCtrls[in.VirtioBlk.Name] = response
	return response, nil
}

// DeleteVirtioBlk deletes a Virtio block device
func (s *Server) DeleteVirtioBlk(_ context.Context, in *pb.DeleteVirtioBlkRequest) (*emptypb.Empty, error) {
	log.Printf("DeleteVirtioBlk: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Name); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// fetch object from the database
	controller, ok := s.VirtioCtrls[in.Name]
	if !ok {
		if in.AllowMissing {
			return &emptypb.Empty{}, nil
		}
		return nil, fmt.Errorf("error finding controller %s", in.Name)
	}
	params := models.NvdaControllerVirtioBlkDeleteParams{
		Name:  in.Name,
		Force: true,
	}
	var result models.NvdaControllerVirtioBlkDeleteResult
	err := s.rpc.Call("controller_virtio_blk_delete", &params, &result)
	if err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	log.Printf("Received from SPDK: %v", result)
	if !result {
		log.Printf("Could not delete: %v", in)
	}
	delete(s.VirtioCtrls, controller.Name)
	return &emptypb.Empty{}, nil
}

// UpdateVirtioBlk updates a Virtio block device
func (s *Server) UpdateVirtioBlk(_ context.Context, in *pb.UpdateVirtioBlkRequest) (*pb.VirtioBlk, error) {
	log.Printf("UpdateVirtioBlk: Received from client: %v", in)

	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.VirtioBlk.Name); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}

	volume, ok := s.VirtioCtrls[in.VirtioBlk.Name]
	if !ok {
		if in.AllowMissing {
			log.Printf("TODO: in case of AllowMissing, create a new resource, don;t return error")
		}
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.VirtioBlk.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	resourceID := path.Base(volume.Name)
	// update_mask = 2
	if err := fieldmask.Validate(in.UpdateMask, in.VirtioBlk); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	log.Printf("TODO: use resourceID=%v", resourceID)
	return nil, status.Errorf(codes.Unimplemented, "UpdateVirtioBlk method is not implemented")
}

// ListVirtioBlks lists Virtio block devices
func (s *Server) ListVirtioBlks(_ context.Context, in *pb.ListVirtioBlksRequest) (*pb.ListVirtioBlksResponse, error) {
	log.Printf("ListVirtioBlks: Received from client: %v", in)
	size, offset, perr := server.ExtractPagination(in.PageSize, in.PageToken, s.Pagination)
	if perr != nil {
		log.Printf("error: %v", perr)
		return nil, perr
	}
	var result []models.NvdaControllerListResult
	err := s.rpc.Call("controller_list", nil, &result)
	if err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	log.Printf("Received from SPDK: %v", result)
	token, hasMoreElements := "", false
	log.Printf("Limiting result len(%d) to [%d:%d]", len(result), offset, size)
	result, hasMoreElements = server.LimitPagination(result, offset, size)
	if hasMoreElements {
		token = uuid.New().String()
		s.Pagination[token] = offset + size
	}
	Blobarray := []*pb.VirtioBlk{}
	for i := range result {
		r := &result[i]
		if r.Type == "virtio_blk" {
			ctrl := &pb.VirtioBlk{
				Name:     server.ResourceIDToVolumeName(r.Name),
				PcieId:   &pb.PciEndpoint{PhysicalFunction: int32(r.PciIndex)},
				VolumeId: &pc.ObjectKey{Value: "TBD"}}
			Blobarray = append(Blobarray, ctrl)
		}
	}
	sortVirtioBlks(Blobarray)
	return &pb.ListVirtioBlksResponse{VirtioBlks: Blobarray}, nil
}

// GetVirtioBlk gets a Virtio block device
func (s *Server) GetVirtioBlk(_ context.Context, in *pb.GetVirtioBlkRequest) (*pb.VirtioBlk, error) {
	log.Printf("GetVirtioBlk: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Name); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// fetch object from the database
	_, ok := s.VirtioCtrls[in.Name]
	if !ok {
		msg := fmt.Sprintf("Could not find Controller: %s", in.Name)
		log.Print(msg)
		return nil, status.Errorf(codes.InvalidArgument, msg)
	}
	var result []models.NvdaControllerListResult
	err := s.rpc.Call("controller_list", nil, &result)
	if err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	log.Printf("Received from SPDK: %v", result)
	resourceID := path.Base(in.Name)
	for i := range result {
		r := &result[i]
		if r.Name == resourceID && r.Type == "virtio_blk" {
			return &pb.VirtioBlk{
				Name:     server.ResourceIDToVolumeName(r.Name),
				PcieId:   &pb.PciEndpoint{PhysicalFunction: int32(r.PciIndex)},
				VolumeId: &pc.ObjectKey{Value: "TBD"}}, nil
		}
	}
	msg := fmt.Sprintf("Could not find Controller: %s", in.Name)
	log.Print(msg)
	return nil, status.Errorf(codes.InvalidArgument, msg)
}

// VirtioBlkStats gets a Virtio block device stats
func (s *Server) VirtioBlkStats(_ context.Context, in *pb.VirtioBlkStatsRequest) (*pb.VirtioBlkStatsResponse, error) {
	log.Printf("VirtioBlkStats: Received from client: %v", in)
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.ControllerId.Value); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}

	var result models.NvdaControllerNvmeStatsResult
	err := s.rpc.Call("controller_virtio_blk_get_iostat", nil, &result)
	if err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	log.Printf("Received from SPDK: %v", result)
	for _, c := range result.Controllers {
		for _, r := range c.Bdevs {
			if r.BdevName == in.ControllerId.Value {
				return &pb.VirtioBlkStatsResponse{Id: in.ControllerId, Stats: &pb.VolumeStats{
					ReadOpsCount:  int32(r.ReadIos),
					WriteOpsCount: int32(r.WriteIos),
				}}, nil
			}
		}
	}
	msg := fmt.Sprintf("Could not find Controller: %s", in.ControllerId.Value)
	log.Print(msg)
	return nil, status.Errorf(codes.InvalidArgument, msg)
}
