package govern_token

import pb "github.com/superconsensus/matrixcore/protos"

type GovManager interface {
	GetGovTokenBalance(accountName string) (*pb.GovernTokenBalance, error)
	DetermineGovTokenIfInitialized() (bool, error)
}
