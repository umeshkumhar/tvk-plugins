package shell

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// TestCase struct is used to create table driven test cases.
// 'In' is passed as input parameter for functions which expect string parameters.
// `Out` is used to compare the output returned by functions with expected output.
type TestCase struct {
	In  []string
	Out []interface{}
}

const (
	baseDir    = "/tmp"
	testDir    = "trillio"
	testFile   = "test"
	unknownDir = "unknown"
)

func prepareTestDirTree(tree string) (string, error) {
	tmpDir, err := ioutil.TempDir("/tmp/", "trillio")
	if err != nil {
		return "", fmt.Errorf("error creating temp directory: %v", err)
	}

	err = os.MkdirAll(filepath.Join(tmpDir, tree), 0755)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", err
	}

	return tmpDir, nil
}

func TestChmod_r(t *testing.T) {
	inputPermBits := "0775"
	testDirPath := "dir/test/"
	t.Logf("Creating temp directory:[%s]", testDirPath)
	tmpDir, dirErr := prepareTestDirTree(testDirPath)
	defer func() {
		t.Logf("Removing temp directory:[%s]", tmpDir)
		err := os.RemoveAll(tmpDir)
		if err != nil {
			log.Warnf("Failed to remove temp directory(%s):=[%s]", tmpDir, err.Error())
		}
		t.Logf("Successfully removed temp directory:[%s]", tmpDir)
	}()
	if dirErr != nil {
		t.Errorf("Unable to create temp dir")
	}
	t.Logf("Successfully created temp directory:[%s]", testDirPath)

	tests := []TestCase{
		{In: []string{tmpDir, inputPermBits}, Out: []interface{}{nil}},
		{In: []string{unknownDir, inputPermBits}, Out: []interface{}{"No such file or directory"}},
	}

	t.Log("Running tests for Chmod_r()")
	for _, test := range tests {
		outStr, err := ChmodR(test.In[0], test.In[1])
		if err != nil {
			if test.Out[0] != nil {
				assert.Contains(t, strings.ToLower(outStr), strings.ToLower(test.Out[0].(string)))
			} else {
				t.Fatalf("ChmodR:Test Case Failed=>[%s]", outStr)
			}
		} else {
			fi, err := os.Stat(test.In[0])
			if err != nil {
				t.Fatalf("Failed:[%s]", err.Error())
			}
			// getting permission bits of file
			outputPermBits := "0" + strconv.FormatUint(uint64(fi.Mode().Perm()), 8)
			assert.Equal(t, inputPermBits, outputPermBits)
		}
	}
}

func TestRmRf(t *testing.T) {
	testFilePath := path.Join(baseDir, testFile)
	testDirPath := path.Join(baseDir, testDir)
	t.Logf("Creating temp directory:[%s]", testDirPath)
	outStr, err := Mkdir(testDirPath)
	defer func() {
		t.Logf("Removing temp directory:[%s]", testDirPath)
		outStr, err = RmRf(testDirPath)
		if err != nil {
			log.Warnf("Failed to remove temp directory(%s):=[%s]", testDirPath, outStr)
		}
		t.Logf("Successfully removed temp directory:[%s]", testDirPath)
	}()

	if err != nil {
		t.Fatalf("failed to create temp directory:[%s]", outStr)
	}
	t.Logf("Successfully created temp directory:[%s]", testDirPath)

	t.Logf("Creating temp file:[%s]", testFilePath)
	outStr, err = CreateFile(testFilePath)
	defer func() {
		t.Logf("Removing temp file:[%s]", testFilePath)
		outStr, err = RmRf(testFilePath)
		if err != nil {
			log.Warnf("Failed to remove temp file(%s):=[%s]", testFilePath, outStr)
		}
		t.Logf("Successfully removed temp file:[%s]", testFilePath)
	}()
	if err != nil {
		t.Fatalf("failed to create temp file:[%s]", outStr)
	}
	t.Logf("Successfully created temp file:[%s]", testFilePath)

	tests := []TestCase{
		{In: []string{testDirPath, baseDir, testDir}, Out: []interface{}{nil}},
		{In: []string{testFilePath, baseDir, testFile}, Out: []interface{}{nil}},
		{In: []string{unknownDir, baseDir, unknownDir}, Out: []interface{}{"no such file or directory"}},
	}

	t.Log("Running tests for RmRf().")
	for _, test := range tests {
		outStr, err := RmRf(test.In[0])
		if err != nil {
			if test.Out[0] != nil {
				assert.Contains(t, strings.ToLower(outStr), strings.ToLower(test.Out[0].(string)))
			} else {
				t.Fatalf("Failed:[%s]", outStr)
			}
		} else {
			isExists, outStr, err := FileExistsInDir(test.In[1], test.In[2])
			if err != nil {
				t.Fatalf("Failed:[%s]", outStr)
			}
			assert.False(t, isExists)
		}
	}
}

func TestMkdir(t *testing.T) {
	testDirPath := path.Join(baseDir, testDir)

	tests := []TestCase{
		{In: []string{testDirPath, baseDir, testDir}, Out: []interface{}{nil}},
	}
	t.Logf("Running tests for Mkdir()")
	for _, test := range tests {
		outStr, err := Mkdir(test.In[0])
		defer func() {
			t.Logf("Removing temp directory:[%s]", testDirPath)
			outStr, err = RmRf(testDirPath)
			if err != nil {
				log.Warnf("Failed to remove temp directory(%s):=[%s]", testDirPath, outStr)
			}
			t.Logf("Successfully removed temp directory:[%s]", testDirPath)
		}()
		if err != nil {
			if test.Out[0] != nil {
				assert.Contains(t, strings.ToLower(outStr), strings.ToLower(test.Out[0].(string)))
			} else {
				t.Fatalf("Failed:[%s]", outStr)
			}
		} else {
			isExists, outStr, err := FileExistsInDir(test.In[1], test.In[2])
			if err != nil {
				t.Fatalf("Failed:[%s]", outStr)
			}
			assert.True(t, isExists)
		}
	}
}

func TestFileExistsInDir(t *testing.T) {
	testDirPath := path.Join(baseDir, testDir)
	t.Logf("Creating temp directory:[%s]", testDirPath)
	outStr, err := Mkdir(testDirPath)
	defer func() {
		t.Logf("Removing temp directory:[%s]", testDirPath)
		outStr, err = RmRf(testDirPath)
		if err != nil {
			log.Warnf("Failed to remove temp directory(%s):=[%s]", testDirPath, outStr)
		}
		t.Logf("Successfully removed temp directory:[%s]", testDirPath)
	}()

	if err != nil {
		t.Fatalf("failed to create temp directory:[%s]", outStr)
	}
	t.Logf("Successfully created temp directory:[%s]", testDirPath)

	tests := []TestCase{
		{In: []string{baseDir, testDir}, Out: []interface{}{nil}},
	}

	t.Logf("Running tests for FileExistsInDir().")
	for _, test := range tests {
		isExists, outStr, err := FileExistsInDir(test.In[0], test.In[1])
		if err != nil {
			if test.Out[0] != nil {
				assert.Contains(t, strings.ToLower(outStr), strings.ToLower(test.Out[0].(string)))
			} else {
				t.Fatalf("Failed:[%s]", outStr)
			}
		} else {
			assert.True(t, isExists)
		}
	}
}

func TestRename(t *testing.T) {
	testDirPath := path.Join(baseDir, testDir)
	testDirPathRenamed := path.Join(baseDir, testDir+"Renamed")
	t.Logf("Creating temp directory:[%s]", testDirPath)
	outStr, err := Mkdir(testDirPath)
	defer func() {
		log.Debugf("Removing temp directory:[%s]", testDirPathRenamed)
		outStr, err = RmRf(testDirPathRenamed)
		if err != nil {
			log.Warnf("Failed to remove temp directory(%s):=[%s]", testDirPathRenamed, outStr)
		}
		log.Infof("Successfully removed temp directory:[%s]", testDirPathRenamed)
	}()
	if err != nil {
		t.Fatalf("failed to create temp directory:[%s]", outStr)
	}
	t.Logf("Successfully created test directory:[%s]", testDirPath)

	tests := []TestCase{
		{In: []string{testDirPath, testDirPathRenamed, baseDir, testDir + "Renamed"}, Out: []interface{}{nil}},
		{In: []string{unknownDir, path.Join(baseDir, unknownDir+"Renamed"), baseDir, unknownDir + "Renamed"},
			Out: []interface{}{"No such file or directory"}},
	}

	t.Logf("Running tests for Rename().")
	for _, test := range tests {
		outStr, err := Rename(test.In[0], test.In[1])
		if err != nil {
			if test.Out[0] != nil {
				assert.Contains(t, strings.ToLower(outStr), strings.ToLower(test.Out[0].(string)))
			} else {
				t.Fatalf("Failed:[%s]", outStr)
			}
		} else {
			isExists, outStr, err := FileExistsInDir(test.In[2], test.In[3])
			if err != nil {
				t.Fatalf("Failed:[%s]", outStr)
			}
			assert.True(t, isExists)
		}
	}
}

func TestRunCmd(t *testing.T) {
	tests := []TestCase{
		{In: []string{"ls"}, Out: []interface{}{nil}},
		{In: []string{"lsd"}, Out: []interface{}{"executable file not found"}},
	}
	t.Logf("Running tests for RunCmd().")
	for _, test := range tests {
		outStruct, err := RunCmd(test.In[0])
		if err != nil {
			if test.Out[0] != nil {
				assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(test.Out[0].(string)))
			} else {
				t.Fatalf("Failed:[%s]", outStruct.Out)
			}
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestCreateFile(t *testing.T) {
	tests := []TestCase{
		{In: []string{"/tmp/trillio", baseDir, "trillio"}, Out: []interface{}{nil}},
	}
	t.Logf("Running tests for CreateFile().")
	for _, test := range tests {
		errStr, err := CreateFile(test.In[0])
		if err != nil {
			if test.Out[0] != nil {
				assert.Contains(t, strings.ToLower(errStr), strings.ToLower(test.Out[0].(string)))
			}
			t.Fatalf("Failed:[%s]", errStr)
		} else {
			isExists, outStr, err := FileExistsInDir(test.In[1], test.In[2])
			if err != nil {
				t.Fatalf("Failed:[%s]", outStr)
			}
			assert.True(t, isExists)
		}
	}
}
