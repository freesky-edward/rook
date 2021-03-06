package longhaul

import (
	"bytes"
	"github.com/rook/rook/tests"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/objects"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"html/template"
	"sync"
	"testing"
)

// Rook Block Storage integration test
// Start MySql database that is using rook provisoned block storage.
// Make sure database is functional

func TestK8sBlockLongHaul(t *testing.T) {
	suite.Run(t, new(K8sBlockLongHaulSuite))
}

type K8sBlockLongHaulSuite struct {
	suite.Suite
	testClient       *clients.TestClient
	bc               contracts.BlockOperator
	kh               *utils.K8sHelper
	initBlockCount   int
	storageClassPath string
	mysqlAppPath     string
	db               *utils.MySQLHelper
	wg               sync.WaitGroup
}

//Test set up - does the following in order
//create pool and storage class, create a PVC, Create a MySQL app/service that uses pvc
func (s *K8sBlockLongHaulSuite) SetupSuite() {
	var err error

	s.testClient, err = clients.CreateTestClient(tests.Platform)
	require.Nil(s.T(), err)

	s.bc = s.testClient.GetBlockClient()
	s.kh = utils.CreatK8sHelper()
	initialBlocks, err := s.bc.BlockList()
	require.Nil(s.T(), err)
	s.initBlockCount = len(initialBlocks)
	s.storageClassPath = "../../../../data/block/storageclass_pool.tmpl"
	s.mysqlAppPath = "../../../../data/integration/mysqlapp.yaml"

	//create storage class
	if scp, _ := s.kh.IsStorageClassPresent("rook-block"); !scp {
		_, _, scs := s.storageClassOperation("mysql-pool", "create")
		require.Equal(s.T(), 0, scs)

		//make sure storageclass is created
		present, err := s.kh.IsStorageClassPresent("rook-block")
		require.Nil(s.T(), err)
		require.True(s.T(), present, "Make sure storageclass is present")
	}
	//create mysql pod
	if _, err := s.kh.GetPVCStatus("mysql-pv-claim"); err != nil {

		s.kh.ResourceOperation("create", s.mysqlAppPath)

		//wait till mysql pod is up
		require.True(s.T(), s.kh.IsPodInExpectedState("mysqldb", "", "Running"))
		pvcStatus, err := s.kh.GetPVCStatus("mysql-pv-claim")
		require.Nil(s.T(), err)
		require.Contains(s.T(), pvcStatus, "Bound")
	}
	dbIP, err := s.kh.GetPodHostID("mysqldb", "")
	require.Nil(s.T(), err)
	//create database connection
	s.db = utils.CreateNewMySQLHelper("mysql", "mysql", dbIP+":30003", "sample")

	require.True(s.T(), s.db.PingSuccess())

	if exist := s.db.TableExists(); !exist {
		s.db.CreateTable()
	}

}

func (s *K8sBlockLongHaulSuite) TestBlockLonghaulRun() {

	s.wg.Add(tests.Env.LoadConcurrentRuns)
	for i := 1; i <= tests.Env.LoadConcurrentRuns; i++ {
		go s.dbOperation(i)
	}
	s.wg.Wait()
}

func (s *K8sBlockLongHaulSuite) dbOperation(i int) {
	defer s.wg.Done()
	//InsertRandomData
	s.db.InsertRandomData()
	s.db.InsertRandomData()
	s.db.InsertRandomData()
	s.db.InsertRandomData()
	s.db.InsertRandomData()
	s.db.InsertRandomData()

	//delete Data
	s.db.DeleteRandomRow()

}
func (s *K8sBlockLongHaulSuite) TearDownSuite() {
	s.db.CloseConnection()
	s.testClient = nil
	s.bc = nil
	s.kh = nil
	s.db = nil
	s = nil
}
func (s *K8sBlockLongHaulSuite) storageClassOperation(poolName string, action string) (string, string, int) {

	t, _ := template.ParseFiles(s.storageClassPath)

	var tpl bytes.Buffer
	config := map[string]string{
		"poolName": poolName,
	}

	t.Execute(&tpl, config)

	cmdStruct := objects.CommandArgs{Command: "kubectl", PipeToStdIn: tpl.String(), CmdArgs: []string{action, "-f", "-"}}

	cmdOut := utils.ExecuteCommand(cmdStruct)

	return cmdOut.StdOut, cmdOut.StdErr, cmdOut.ExitCode

}
