package propose

import (
	pb "github.com/superconsensus/matrixcore/protos"
)

type ProposeManager interface {
	GetProposalByID(proposalID string) (*pb.Proposal, error)
}
