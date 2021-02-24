package test

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/trilioData/k8s-triliovault/internal/utils"
	"github.com/trilioData/k8s-triliovault/internal/utils/shell"

	"github.com/joshlf/go-acl"
	"github.com/pkg/xattr"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/util/json"
)

// Loop device Id's
var (
	SourceLoopDeviceID    string
	DataStoreLoopDeviceID string
)

const (
	CreateOp string = "create"
	DeleteOp string = "delete"
	UpdateOp string = "update"
)

// Losetup struct contains fields and methods for setting loop devices which is required
// for running dataMover unit tests.
type Losetup struct {
	dirPath, loopDataStoreNumber, loopDeviceNumber string
	srcLoopDevice, dataStoreLoopDevice             map[string]string
}

var SrcLoopDevice = map[string]string{
	"devicePath":   path.Join(utils.BaseDir, utils.LoopDirName, utils.SrcFile),
	"backing-file": path.Join(utils.BaseDir, utils.LoopDirName, utils.SrcFile),
	"dataSize":     utils.SrcDiskSize,
}

var DataStoreLoopDevice = map[string]string{
	"devicePath":   path.Join(utils.BaseDir, utils.LoopDirName, utils.BlankFile),
	"mountpoint":   path.Join(utils.BaseDir, utils.LoopDirName, "datastore"),
	"backing-file": path.Join(utils.BaseDir, utils.LoopDirName, utils.BlankFile),
}

var RestoreLoopDevice = map[string]string{
	"devicePath":   path.Join(utils.BaseDir, utils.LoopDirName, utils.RestoreSampleData),
	"backing-file": path.Join(utils.BaseDir, utils.LoopDirName, utils.RestoreSampleData),
}

var RestoreLoopDeviceTest = map[string]string{
	"devicePath":   path.Join(utils.BaseDir, utils.LoopDirName, utils.RestoreSampleDataTest),
	"backing-file": path.Join(utils.BaseDir, utils.LoopDirName, utils.RestoreSampleDataTest),
}

// Init initializes required fields which are related to loop device setup.
func (lo *Losetup) Init() {
	lo.dirPath = path.Join(utils.BaseDir, utils.LoopDirName)
	lo.loopDeviceNumber = SourceLoopDeviceID
	lo.loopDataStoreNumber = DataStoreLoopDeviceID
	lo.srcLoopDevice = SrcLoopDevice
	lo.dataStoreLoopDevice = DataStoreLoopDevice
}

// SetupBaseDirectory creates the base directory for data mover unit tests.
// output=>outStr: returns stdout.
// 		   err: non-nil error if command execution failed.
func (lo *Losetup) SetupBaseDirectory() (string, error) {
	log.Infof("Setting up Base Directory for Data Mover Unit testing...")
	outStr, err := shell.Mkdir(lo.dirPath)
	if err != nil {
		log.Errorf("Failed to Setup Base Directory (%s) => [%s]", lo.dirPath, outStr)
		return outStr, err
	}
	log.Infof("Base Directory Created for Data Mover Unit testing => [%s]", lo.dirPath)
	return outStr, nil
}

// CreateSampleData used to create sample test data required for data mover unit testing.
// input=>
// 	_if        : input file
// 	of  		  : output file
// 	count 	  : no of blocks
// 	filesystem : true(if device needs mounting)/false(if device doesn't need mounting)
// output=>outStruct.out:stderr if command execution fails else stdout.
// 		   err: non-nil error if command execution failed.
func (lo *Losetup) CreateSampleData(inputFile, outputFile, count string, fileSystem bool) (string, error) {
	cmd := fmt.Sprintf("dd if=%s of=%s bs=%s count=%s", inputFile, outputFile, utils.BlockSize, count)
	log.Debugf("Command:=[%s], FileSystem:=[%t]", cmd, fileSystem)
	outStruct, err := shell.RunCmd(cmd)

	if err != nil {
		log.Errorf("Failed to create test data(%s), Command:[%s] => (FAILED) %s, with exit code: %d",
			outputFile, cmd, outStruct.Out, outStruct.ExitCode)
		return outStruct.Out, err
	}

	log.Infof("Successfully created test data(%s),Command:[%s] => (Success) %s, with exit code: %d",
		outputFile, cmd, outStruct.Out, outStruct.ExitCode)

	if fileSystem {
		outStr, err := lo.SetFileSystemToLoopDevice(outputFile)
		if err != nil {
			log.Errorf("Failed to setup file system to loop device (%s)=>[%s]", outputFile, outStr)
			return outStr, err
		}
	}
	return "", nil
}

// CreateLoopDevice creates new loop device with specified deviceID.
// input=>deviceID:loop device number
// output=>outStruct.out:stderr if command execution fails else stdout.
// 		   err: non-nil error if command execution failed.
func (lo *Losetup) CreateLoopDevice(deviceID string) (string, error) {
	log.Infof("Loop Device (/dev/loop%s) Creation is in progress...", deviceID)
	cmd := fmt.Sprintf("mknod -m660 /dev/loop%s b 7 %s", deviceID, deviceID)
	log.Debugf("Loop Device Creation Command:%s", cmd)
	outStruct, err := shell.RunCmd(cmd)
	if err != nil {
		log.Errorf("Failed To Create Loop Device(/dev/loop%s), Command=>[%s], => (%s)", deviceID, cmd, outStruct.Out)
		return outStruct.Out, err
	}
	log.Infof("Successfully Created Loop device(/dev/loop%s),Command:[%s]", deviceID, cmd)
	return outStruct.Out, nil
}

// LoopSetup attach loop device to already created file.
// input=>loopInfo:contains loop device info
// output=>outStr: returns stdout.
// 		   outStruct.out:stderr if command execution fails else stdout.
// 		   err: non-nil error if command execution failed.
func (lo *Losetup) LoopSetup(loopInfo map[string]string) (string, error) {
	log.Infof("Loop Device Setup is in Progress.loopDevice=>[%s],Backing-file=>[%s]", loopInfo["devicePath"], loopInfo["backing-file"])
	if _, ok := loopInfo["mountpoint"]; ok {
		outStr, err := shell.Mkdir(loopInfo["mountpoint"])
		if err != nil {
			log.Errorf("Failed to create mount directory(%s) for [%s] => (%s)", loopInfo["mountpoint"], loopInfo["devicePath"], outStr)
			return "", err
		}
		log.Infof("Created mount directory(%s) for [%s]", loopInfo["mountpoint"], loopInfo["devicePath"])
	}
	cmd := fmt.Sprintf("losetup -P %s %s", loopInfo["devicePath"], loopInfo["backing-file"])
	log.Debugf("LoopSetup Command:[%s]", cmd)
	outStruct, err := shell.RunCmd(cmd)
	if err != nil {
		log.Errorf("Failed to setup loop device (%s),Command=> [%s] => (%s)", loopInfo["devicePath"], cmd, outStruct.Out)
		return outStruct.Out, err
	}
	log.Infof("Successfully setup loop device(%s),Command:[%s]", loopInfo["devicePath"], cmd)
	return outStruct.Out, nil
}

// SetFileSystemToLoopDevice formats loop device with ext4.
// input=>backingfile: data file name.
// output=>outStruct.out:stderr if command execution fails else stdout.
// 		   err: non-nil error if command execution failed.
func (lo *Losetup) SetFileSystemToLoopDevice(backingfile string) (string, error) {
	log.Infof("Setting Up File System to backing file := (%s)", backingfile)

	cmd := fmt.Sprintf("mkfs.ext4 %s", backingfile)

	log.Debugf("FileSystem Setting Command:[%s]", cmd)

	outStruct, err := shell.RunCmd(cmd)
	if err != nil {
		log.Errorf("Failed to set file system to backing file: [%s], Command:[%s] => (%s)", backingfile, cmd, outStruct.Out)
		return outStruct.Out, err
	}
	log.Infof("Successfully set file system to backing file:[%s]", backingfile)

	return outStruct.Out, nil
}

// Mount mounts loop device to specified path.
// input=>loopInfo:contains loop device info
// output=>outStruct.out:stderr if command execution fails else stdout.
// 		   err: non-nil error if command execution failed.
func (lo *Losetup) Mount(loopInfo map[string]string) (string, error) {
	log.Infof("Mounting file system is in progress.device(%s)=>mountpoint(%s)", loopInfo["devicePath"], loopInfo["mountpoint"])
	cmd := fmt.Sprintf("mount -o loop %s %s", loopInfo["devicePath"], loopInfo["mountpoint"])
	log.Debugf("Mount Command:[%s]", cmd)
	outStruct, err := shell.RunCmd(cmd)
	if err != nil {
		log.Errorf("Failed to mount file system of device(%s),Command:[%s]=>[%s]", loopInfo["devicePath"], cmd, outStruct.Out)
		return outStruct.Out, err
	}
	log.Infof("Successfully mounted file system of(%s)=>[%s]", loopInfo["devicePath"], loopInfo["mountpoint"])
	return outStruct.Out, nil
}

// ConvertData converts existing data by count blocks.
// input=>of   : output file
// 		  count: no of blocks
// output=>outStr: returns stdout.
// 		   err.Error(): returns error string if command execution fails.
// 		   err   : non-nil error if command execution failed.
func (lo *Losetup) ConvertData(of, count string) (string, error) {
	log.Infof("Data Conversion of [%s] by [%s]M is in progress.", of, count)
	// suppress linter here issue -> G204: Subprocess launched with function call as argument or cmd arguments
	//nolint:gosec // no other options here
	cmd := exec.Command("dd", "if=/dev/urandom", "of="+of, "conv=notrunc", "bs="+utils.BlockSize, "count="+count)
	log.Debugf("Data Conversion Command:%s", cmd.Args)
	outStr, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to convert data of [%s],Command:[%s]=>[%s]", of, cmd.Args, err.Error())
		return err.Error(), err
	}
	log.Infof("Successfully converted data of [%s] by [%s],Command:[%s]", of, count, cmd.Args)
	return string(outStr), nil
}

// unmount unmounts the specified mountpoint.
// input=>mountpoint: mounted directory path.
// output=>outStruct.out:stderr if command execution fails else stdout.
// 		   err: non-nil error if command execution failed.
func (lo *Losetup) unmount(mountpoint string) (string, error) {
	log.Infof("Unmounting file system:[%s]", mountpoint)
	cmd := fmt.Sprintf("umount %s", mountpoint)
	log.Debugf("Unmount Command:[%s]", cmd)
	outStruct, err := shell.RunCmd(cmd)
	if err != nil {
		log.Errorf("Failed to unmount:[%s],Command:[%s] => (%s)", mountpoint, cmd, outStruct.Out)
		return outStruct.Out, err
	}
	log.Infof("Successfully unmounted file system:[%s],Command:[%s]", mountpoint, cmd)
	return outStruct.Out, nil
}

// CreateQcow2 creates the images using qemu-img according to the passed params
// params:
// input=> format: qcow2 format string.
// 		   srcPath: source pv path to create qcow2.
// 		   destPath: path to store created qcow2.
// output=> outStruct.Out: stderr if command execution fails else stdout
// 			err: non-nil error if command execution failed.
func CreateQcow2(format, srcPath, destPath string) (string, error) {
	cmd := fmt.Sprintf("qemu-img convert -O %s %s %s", format, srcPath, destPath)
	log.Debugf("Create Image command(Qemu):[%s]", cmd)

	outStruct, err := shell.RunCmd(cmd)
	if err != nil {
		log.Errorf("Command:[%s] => (FAILED) %s, with exit code: %d", cmd, outStruct.Out, outStruct.ExitCode)
		return outStruct.Out, err
	}

	log.Debugf("Command:[%s] => (Success) %s, with exit code: %d", cmd, outStruct.Out, outStruct.ExitCode)
	return outStruct.Out, nil
}

// CreateQcow2WithBacking creates the images using qemu-img with the backing chain reference.
// params:
// input=format: qcow2 format string.
// 		 srcPath: source pv path to create qcow2.
// 		 prevQcow2Path: prev qcow2 path to use as a backing image for current image.
// 		 intermediateQcow2Path: path to store create qcow2
// output:outStruct.Out: stderr if command execution fails else stdout
// 		  err: non-nil error if command execution failed.
func CreateQcow2WithBacking(format, srcPath, prevQcow2Path, intermediateQcow2Path string) (string, error) {
	cmd := fmt.Sprintf("qemu-img convert -O %s %s -B %s %s", format, srcPath, prevQcow2Path, intermediateQcow2Path)
	log.Debugf("Create Image command(Qemu):[%s]", cmd)

	outStruct, err := shell.RunCmd(cmd)
	if err != nil {
		log.Errorf("Command:[%s] => (FAILED)%s, with exit code: %d", cmd, outStruct.Out, outStruct.ExitCode)
		return outStruct.Out, err
	}
	log.Debugf("Diff Command:[%s] => (Success) %s, with exit code: %d", cmd, outStruct.Out, outStruct.ExitCode)
	return outStruct.Out, nil
}

// CreateSampleBackup: helper function to create sample backup files need for unit testing.
// input=>bkpName: name given to final backup path.
// output=>outStr: returns stdout.
// 		   err: non-nil error if command execution failed.
func CreateSampleBackup(bkpName string) (string, error) {
	log.Infof("Creating sample backup file:[%s]...", bkpName)
	destPath := path.Join(utils.BaseDir, utils.LoopDirName, "bkp1", bkpName)

	outStr, err := shell.Mkdir(path.Join(utils.BaseDir, utils.LoopDirName, "bkp1"))
	if err != nil {
		log.Errorf("Failed to created backup directory(%s):[%s]", destPath, outStr)
		return outStr, err
	}

	log.Infof("Created test backup directory:[%s]", bkpName)
	srcData := path.Join(utils.BaseDir, utils.LoopDirName, "bkp1", utils.SrcFile)
	overlayData := path.Join(utils.BaseDir, utils.LoopDirName, "bkp1", utils.Overlay)
	lo := Losetup{}
	outStr, err = lo.CreateSampleData(utils.RandomDataInputFile, srcData, utils.SrcDiskSize, false)
	if err != nil {
		log.Errorf("Failed to create data:[%s] => (%s)", srcData, outStr)
		return outStr, err
	}
	cmd := exec.Command("cp", srcData, overlayData)
	log.Debugf("Copy Command:%s", cmd.Args)
	_, err = cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to copy file(%s => %s),Command:[%s]=>%s", srcData, overlayData, cmd.Args, err.Error())
		return err.Error(), err
	}

	outStr, err = CreateQcow2(utils.Qcow2Format, srcData, destPath)

	if err != nil {
		log.Errorf("Failed to create sample backup:[%s]", outStr)
		return outStr, err
	}
	log.Infof("Successfully created datamover sample backup:[%s]", bkpName)
	return "", nil
}

// ConvertBytesToMb convert given bytes to mb.
// input=>bytes: bytes to convert.
// output=>returns mb
func ConvertBytesToMb(bytes interface{}) int {
	var mb int
	switch size := bytes.(type) {
	case int64:
		mb = int((size / 1024) / 1024)
	case float64:
		mb = int((size / 1024.0) / 1024.0)
	}
	return mb
}

// GetImageInfo parses the specfied qemu-img.
// input=>imagePath: source qemu-img path for parsing.
// output=>imageInfo: contains parsed image info.
// 		   errStr: returns stdout if command successfully executes else return stderr.
// 		   err: non-nil error if command execution failed.
func GetImageInfo(imagePath string) (imageInfo map[string]string, errStr string, err error) {
	imageInfo = map[string]string{
		"size":             "0",
		"backing-filename": "",
	}
	cmd := fmt.Sprintf("qemu-img info %s --output json", imagePath)
	outStruct, err := shell.RunCmd(cmd)
	if err != nil {
		log.Errorf("Failed:%s", outStruct.Out)
		errStr = outStruct.Out
		return imageInfo, errStr, err
	}
	outputMap := make(map[string]interface{})
	err = json.Unmarshal([]byte(outStruct.Out), &outputMap)
	if err != nil {
		log.Errorf("Failed:[%s]", err.Error())
		errStr = err.Error()
		return imageInfo, errStr, err
	}

	imageInfo["size"] = strconv.Itoa(ConvertBytesToMb(outputMap["actual-size"]))

	if _, ok := outputMap["backing-filename"]; ok {
		imageInfo["backing-filename"] = outputMap["backing-filename"].(string)
	}
	return imageInfo, errStr, err
}

func GetDirSize(dirPath string) (int, error) {
	var size int64
	err := filepath.Walk(dirPath, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	mb := (int(size) / 1024.0) / 1024.0
	return mb, err
}

// BackupTestSetup Creates test setup for datamover raw device testing.
// output=>outStr: returns stdout.
// 		   err: non-nil error if command execution failed.
func BackupTestSetup() (string, error) {
	log.Infof("DataMover Test Backup setup started...")
	outStr, err := CreateSampleBackup(utils.Qcow2PV) // Sample backup for data mover utility funcions
	if err != nil {
		log.Errorf("Failed to create  sample backup:[%s]", outStr)
		return outStr, err
	}
	lo := Losetup{}
	// Data modification for calculating diff
	outStr, err = lo.ConvertData(path.Join(utils.BaseDir, utils.LoopDirName, "bkp1", utils.Overlay), utils.SrcDataModificationSize)
	if err != nil {
		log.Errorf("Failed to covert data:[%s]", outStr)
		return outStr, err
	}
	prevQcow2Path := path.Join(utils.BaseDir, utils.LoopDirName, "bkp1", utils.Qcow2PV)
	intermediateQcow2Path := path.Join(utils.BaseDir, utils.LoopDirName, "bkp1", utils.IntermediateQcow2PV)
	// Qcow2 with backing creation needed for diff test function
	outStr, err = CreateQcow2WithBacking(utils.Qcow2Format,
		path.Join(utils.BaseDir, utils.LoopDirName, "bkp1", utils.Overlay),
		prevQcow2Path, intermediateQcow2Path)
	if err != nil {
		log.Errorf("Failed to create  qcow2 with backing:[%s]", outStr)
		return outStr, err
	}
	return outStr, nil
}

// FileSystemSetup Creates test setup for datamover file system unit testing.
// output=>outStr: returns stdout.
// 		   err: non-nil error if command execution failed.
func FileSystemSetup() (string, error) {
	log.Infof("DataMover File system unit test setup started...")
	var (
		outStr   string
		err      error
		reqPaths []string
	)
	reqPaths = append(reqPaths, path.Join(utils.BaseDir, utils.LoopDirName, "fsData"),
		path.Join(utils.BaseDir, utils.LoopDirName, "fsBackUp", "full"),
		path.Join(utils.BaseDir, utils.LoopDirName, "fsRestore", "full"),
		path.Join(utils.BaseDir, utils.LoopDirName, "fsBackUp", "incremental"),
		path.Join(utils.BaseDir, utils.LoopDirName, "fsRestore", "incremental"),
		path.Join(utils.BaseDir, utils.LoopDirName, "oldWayFs", "fsData"),
		path.Join(utils.BaseDir, utils.LoopDirName, "oldWayFs", "fsBackUp", "full"),
		path.Join(utils.BaseDir, utils.LoopDirName, "oldWayFs", "fsBackUp", "incremental"),
		path.Join(utils.BaseDir, utils.LoopDirName, "oldWayFs", "fsRestore", "incremental"),
	)
	for _, rP := range reqPaths {
		outStr, err = shell.Mkdir(rP)
		if err != nil {
			log.Errorf("Failed to create test file system directory:[%s] => (%s)", rP, outStr)
			return outStr, err
		}
		log.Infof("Created Test file system directory(%s).", rP)
	}

	return outStr, nil
}

// SourceDeviceSetup Creates test setup for datamover raw device unit testing.
// output=>outStr: returns stdout.
// 		   err: non-nil error if command execution failed.
func (lo *Losetup) SourceDeviceSetup() (outStr string, err error) {
	sourceImagePath := path.Join(lo.dirPath, utils.SrcFile)
	outStr, err = lo.CreateSampleData(utils.RandomDataInputFile, sourceImagePath, utils.SrcDiskSize, false)
	if err != nil {
		return outStr, err
	}
	return outStr, err
}

// DataSourceSetup Creates data store test setup for datamover unit testing.
// output=>outStr: returns stdout.
// 		   err: non-nil error if command execution failed.
func (lo *Losetup) DataSourceSetup() (outStr string, err error) {
	// DataStore base image for loop device virtual size
	datastoreImagePath := path.Join(lo.dirPath, utils.BlankFile)
	outStr, err = lo.CreateSampleData(utils.BlankInputFile, datastoreImagePath, utils.DataStoreSize, true)
	if err != nil {
		return outStr, err
	}

	outStr, err = shell.Mkdir(DataStoreLoopDevice["mountpoint"])
	if err != nil {
		log.Errorf("Failed to create mount directory(%s) for [%s] => (%s)",
			DataStoreLoopDevice["mountpoint"], DataStoreLoopDevice["devicePath"], outStr)
		return "", err
	}
	log.Infof("Created mount directory(%s) for [%s]", DataStoreLoopDevice["mountpoint"], DataStoreLoopDevice["devicePath"])

	outStr, err = lo.Mount(DataStoreLoopDevice) // Mountpoint creation for datastore
	return outStr, err
}

// RestoreDeviceSetup Creates datamover unit test setup for restore unit testing.
// output=>outStr: returns stdout.
// 		   err: non-nil error if command execution failed.
func (lo *Losetup) RestoreDeviceSetup() (outStr string, err error) {
	// Restore base image for loop device virtual size
	outStr, err = lo.CreateSampleData(utils.BlankInputFile, path.Join(lo.dirPath,
		utils.RestoreSampleData), utils.RestoreVirtualSize, false)
	if err != nil {
		return outStr, err
	}

	// Restore base image for test loop device virtual size
	outStr, err = lo.CreateSampleData(utils.BlankInputFile, path.Join(lo.dirPath, utils.RestoreSampleDataTest),
		utils.RestoreVirtualSize, false)
	if err != nil {
		return outStr, err
	}

	return outStr, err
}

// DataMoverUnitTestSetup Creates datamover unit test setup.
// output=>outStr: returns stdout.
// 		   err: non-nil error if command execution failed.
func (lo *Losetup) DataMoverUnitTestSetup() (outStr string, err error) {
	log.Infof("Setting DataMover Unit Test Setup Started...")
	lo.Init()

	outStr, err = lo.SetupBaseDirectory()
	if err != nil {
		return outStr, err
	}

	// Source data image for loop device Actual disk size
	outStr, err = lo.SourceDeviceSetup()
	if err != nil {
		return outStr, err
	}

	outStr, err = lo.DataSourceSetup()
	if err != nil {
		return outStr, err
	}

	outStr, err = lo.RestoreDeviceSetup()
	if err != nil {
		return outStr, err
	}

	outStr, err = BackupTestSetup()
	if err != nil {
		return outStr, err
	}

	outStr, err = FileSystemSetup()
	if err != nil {
		return outStr, err
	}
	log.Infof("Successfully created datamover unit test setup.")
	return outStr, nil
}

// TearDownSetup tear down's data mover unit test setup.
// output=>outStr: returns stdout.
// 		   err: non-nil error if command execution failed.
func (lo *Losetup) TearDownSetup() (outStr string, err error) {
	log.Infof("DataMover Unit test setup tear down started...")

	outStr, err = lo.unmount(DataStoreLoopDevice["mountpoint"]) // unmount data store mountpoint
	if err != nil {
		return outStr, err
	}

	outStr, err = shell.RmRf(path.Join(utils.BaseDir, utils.LoopDirName)) // remove datamover unit test base directory
	if err != nil {
		return outStr, err
	}

	log.Infof("Successfully tear down data mover unit test setup.")
	return outStr, err
}

func CreateActualFSData(backupType, fsPath string) error {
	createFSContent, _ := getFSContent(backupType, fsPath)
	// Create Dirs
	dirs := createFSContent["Dirs"].([]map[string]string)
	for _, dir := range dirs {
		switch dir["action"] {
		case CreateOp:
			mkErr := os.MkdirAll(dir["fullPath"], os.ModePerm)
			if mkErr != nil {
				log.Errorf("Error while creating dir %s: %+v", dir["fullPath"], mkErr)
				return mkErr
			}
		case DeleteOp:
			rmErr := os.RemoveAll(dir["fullPath"])
			if rmErr != nil {
				log.Errorf("Error while removing dir %s: %+v", dir["fullPath"], rmErr)
				return rmErr
			}
		}
		setErr := setXAttrAndACL(dir)
		if setErr != nil {
			log.Errorf("Error while setting xattr and ACL for %s: %+v", dir["fullPath"], setErr)
			return setErr
		}
	}
	// Create Files
	files := createFSContent["Files"].([]map[string]string)
	for _, f := range files {
		switch f["action"] {
		case CreateOp:
			// Create file and write content
			fi, crErr := os.Create(f["fullPath"])
			if crErr != nil {
				log.Errorf("Error while creating file %s: %+v", f["fullPath"], crErr)
				return crErr
			}
			_, wErr := fi.Write([]byte(f["content"]))
			if wErr != nil {
				log.Errorf("Error while writing file %s: %+v", f["fullPath"], wErr)
				return wErr
			}
			_ = fi.Close()
		case DeleteOp:
			rmErr := os.RemoveAll(f["fullPath"])
			if rmErr != nil {
				log.Errorf("Error while removing file %s: %+v", f["fullPath"], rmErr)
				return rmErr
			}
		case UpdateOp:
			if f["content"] != "" {
				fi, opErr := os.OpenFile(f["fullPath"], os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if opErr != nil {
					log.Errorf("Error while opening file %s: %s", f["fullPath"], opErr)
					return opErr
				}
				_, aErr := fi.WriteString(f["content"])
				if aErr != nil {
					log.Errorf("Error while appending content in file %s: %s", f["fullPath"], aErr)
					return aErr
				}
				_ = fi.Close()
			}
		}
		setErr := setXAttrAndACL(f)
		if setErr != nil {
			log.Errorf("Error while setting xattr and ACL for %s: %+v", f["fullPath"], setErr)
			return setErr
		}
	}
	// Create Links
	links := createFSContent["Links"].([]map[string]string)
	for _, l := range links {
		switch l["action"] {
		case CreateOp:
			sErr := os.Symlink(l["target"], l["fullPath"])
			if sErr != nil {
				log.Errorf("Error while creating symlink %s: %+v", l["fullPath"], sErr)
				return sErr
			}
		case DeleteOp:
			rmErr := os.RemoveAll(l["fullPath"])
			if rmErr != nil {
				log.Errorf("Error while removing symlink %s: %+v", l["fullPath"], rmErr)
				return rmErr
			}
		}
	}

	return nil
}

// nolint:dupl // This function is giving duplicate lint error for for dirs and files validation code.
// This code going to be change once we consider big data set
func ValidateActualFSData(backupType, fsPath string, checkAttributes bool) error {
	_, validateFSContent := getFSContent(backupType, fsPath)
	dirs := validateFSContent["Dirs"].([]map[string]string)
	for _, dir := range dirs {
		switch dir["action"] {
		case CreateOp:
			if _, statErr := os.Stat(dir["fullPath"]); statErr != nil {
				log.Errorf("Path should be present. Error path doesn't "+
					"exists %s: %+v", dir["fullPath"], statErr)
				return statErr
			}
		case DeleteOp:
			if _, statErr := os.Stat(dir["fullPath"]); statErr == nil {
				log.Errorf("Path %s should not be present", dir["fullPath"])
				return fmt.Errorf("path %s should not be present", dir["fullPath"])
			}
		}
		if checkAttributes {
			valErr := validateXAttrAndACL(dir, true)
			if valErr != nil {
				log.Errorf("Error while validating XAttr and ACL for %s: %+v", dir["fullPath"], valErr)
				return valErr
			}
		}
	}
	files := validateFSContent["Files"].([]map[string]string)
	for _, f := range files {
		switch f["action"] {
		case CreateOp:
			if _, statErr := os.Stat(f["fullPath"]); statErr != nil {
				log.Errorf("Path should be present. Error path doesn't "+
					"exists %s: %+v", f["fullPath"], statErr)
				return statErr
			}
		case DeleteOp:
			if _, statErr := os.Stat(f["fullPath"]); statErr == nil {
				log.Errorf("File %s should not be present ", f["fullPath"])
				return fmt.Errorf("file %s should not be present ", f["fullPath"])
			}
		}
		if checkAttributes {
			valErr := validateXAttrAndACL(f, false)
			if valErr != nil {
				log.Errorf("Error while validating XAttr and ACL for %s: %+v", f["fullPath"], valErr)
				return valErr
			}
		}
	}
	links := validateFSContent["Links"].([]map[string]string)
	for _, l := range links {
		switch l["action"] {
		case CreateOp:
			if _, statErr := os.Lstat(l["fullPath"]); statErr != nil {
				log.Errorf("Path should be present. Error path doesn't "+
					"exists %s: %+v", l["fullPath"], statErr)
				return statErr
			}
			// Link should point to the required target
			tgt, rErr := os.Readlink(l["fullPath"])
			if rErr != nil {
				log.Errorf("Error while reading link %s: %+v", l["fullPath"], rErr)
				return rErr
			}
			if tgt != l["target"] {
				log.Errorf("Link validation for %s failed. Expected target is %s but "+
					"the actual target is %s", l["fullPath"], l["target"], tgt)
				return fmt.Errorf("link validation for %s failed", l["fullPath"])
			}
		case DeleteOp:
			if _, statErr := os.Lstat(l["fullPath"]); statErr == nil {
				log.Errorf("Link %s should not be present", l["fullPath"])
				return fmt.Errorf("link %s should not be present", l["fullPath"])
			}
		}
	}

	return nil
}

func getFSContent(backupType, dst string) (createContent, validateContent map[string]interface{}) {
	createFSContent := map[string]interface{}{
		"Full": map[string]interface{}{
			"Dirs": []map[string]string{
				{
					"action":   "create",
					"fullPath": path.Join(dst, "oneDir"),
					"xattr":    "name oneDir",
					"dacl":     "u:1001:rwx,g:8001:rw",
					"acl":      "u:1002:rw,g:8002:rwx",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "oneDir", "ondDirA1"),
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "oneDir", "ondDirB1"),
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "oneDir", "ondDirA1", "oneDirA2"),
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "twoDir"),
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "twoDir", "twoDirA1"),
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "twoDir", "twoDirB1"),
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "twoDir", "twoDirB1", "twoDirB2"),
					"xattr":    "name twoDirB2",
					"acl":      "u:1003:rw,g:8003:rwx",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "zDir"),
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, ".hiddenDir"),
				},
			},
			"Files": []map[string]string{
				{
					"action":   "create",
					"fullPath": path.Join(dst, "oneDir", "ondDirA1", "oneFileA1.txt"),
					"xattr":    "name oneFileA1.txt",
					"acl":      "u:2001:rwx,g:9001:rw",
					"content":  "Sample content with symbols @@@@####",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "twoDir", "twoDirB1", "twoDirB2", "twoFileB2.txt"),
					"xattr":    "name twoFileB2.txt",
					"acl":      "u:2002:rw,g:9002:rw",
					"content":  "Sample content 0123456789",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "twoDir", "twoDirA1", "twoFileA1.txt"),
					"content":  "Sample content [Trilio] [DataBackupRestore]",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "zDir", "zDirFile"),
					"content":  "I am the zDirFile.",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, ".hiddenFile"),
					"content":  "Hidden file/dir name starts with .",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "My file for test"),
					"content":  "File with spaces in the name",
				},
			},
			"Links": []map[string]string{
				{
					"action":   "create",
					"target":   path.Join("twoDir", "twoDirA1"),
					"fullPath": path.Join(dst, "lnDir"),
				},
				{
					"action":   "create",
					"target":   path.Join("ondDirA1", "oneFileA1.txt"),
					"fullPath": path.Join(dst, "oneDir", "oneDirLn"),
				},
				{
					"action":   "create",
					"target":   path.Join("zDirFile"),
					"fullPath": path.Join(dst, "zDir", "zDirLn"),
				},
			},
		},
		"Incremental": map[string]interface{}{
			"Dirs": []map[string]string{
				{
					"action":   "create",
					"fullPath": path.Join(dst, "threeDir"),
					"xattr":    "Created in incremental, name threeDir",
					"dacl":     "u:1004:rwx,g:8004:rw",
					"acl":      "u:1005:rw,g:8005:rwx",
				},
				{
					"action":   "delete",
					"fullPath": path.Join(dst, "oneDir", "ondDirA1", "oneDirA2"),
				},
				{
					"action":   "update",
					"fullPath": path.Join(dst, "oneDir"),
					"xattr":    "Updated in incremental, name oneDir",
					"dacl":     "u:1006:rwx,g:8006:rw",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "twoDir", "twoDirC1"),
				},
			},
			"Files": []map[string]string{
				{
					"action":   "delete",
					"fullPath": path.Join(dst, ".hiddenFile"),
				},
				{
					"action":   "update",
					"fullPath": path.Join(dst, "twoDir", "twoDirB1", "twoDirB2", "twoFileB2.txt"),
					"xattr":    "name twoFileB2.txt",
					"acl":      "u:2003:rw,g:9003:rw",
					"content":  " I am the update!!",
				},
				{
					"action":   "update",
					"fullPath": path.Join(dst, "twoDir", "twoDirA1", "twoFileA1.txt"),
					"acl":      "u:2004:rw,g:9004:rw",
					"xattr":    "name twoFileA1.txt",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "twoDir", "twoDirA1", "supernew"),
					"content":  "Sample new content",
					"xattr":    "I am super new file",
				},
				{
					"action":   "update",
					"fullPath": path.Join(dst, "zDir", "zDirFile"),
					"content":  " I am the update in zDirFile",
				},
			},
			"Links": []map[string]string{
				{
					"action":   "delete",
					"fullPath": path.Join(dst, "lnDir"),
				},
				{
					"action":   "create",
					"target":   path.Join("twoDir", "twoDirA1", "supernew"),
					"fullPath": path.Join(dst, "lnsuper"),
				},
				{
					"action":   "create",
					"target":   path.Join("oneDir", "oneDirLn"),
					"fullPath": path.Join(dst, "topDirOneLn"),
				},
				{
					"action":   "create",
					"target":   path.Join("zDir", "zDirLn"),
					"fullPath": path.Join(dst, "topDirZLn"),
				},
			},
		},
	}

	// Club the above Full and Incremental FS content to verify
	// it after incremental backup and restore
	validateFSContent := map[string]interface{}{
		"Incremental": map[string]interface{}{
			"Dirs": []map[string]string{
				{
					"action":   "create",
					"fullPath": path.Join(dst, "oneDir"),
					"xattr":    "Updated in incremental, name oneDir",
					"dacl":     "u:1001:rwx,g:8001:rw,u:1006:rwx,g:8006:rw",
					"acl":      "u:1002:rw,g:8002:rwx",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "oneDir", "ondDirA1"),
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "oneDir", "ondDirB1"),
				},
				{
					"action":   "delete",
					"fullPath": path.Join(dst, "oneDir", "ondDirA1", "oneDirA2"),
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "twoDir"),
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "twoDir", "twoDirA1"),
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "twoDir", "twoDirB1"),
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "twoDir", "twoDirB1", "twoDirB2"),
					"xattr":    "name twoDirB2",
					"acl":      "u:1003:rw,g:8003:rwx",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, ".hiddenDir"),
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "threeDir"),
					"xattr":    "Created in incremental, name threeDir",
					"dacl":     "u:1004:rwx,g:8004:rw",
					"acl":      "u:1005:rw,g:8005:rwx",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "zDir"),
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "twoDir", "twoDirC1"),
				},
			},
			"Files": []map[string]string{
				{
					"action":   "create",
					"fullPath": path.Join(dst, "oneDir", "ondDirA1", "oneFileA1.txt"),
					"xattr":    "name oneFileA1.txt",
					"acl":      "u:2001:rwx,g:9001:rw",
					"content":  "Sample content with symbols @@@@####",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "twoDir", "twoDirB1", "twoDirB2", "twoFileB2.txt"),
					"xattr":    "name twoFileB2.txt",
					"acl":      "u:2002:rw,g:9002:rw,u:2003:rw,g:9003:rw",
					"content":  "Sample content 0123456789 I am the update!!",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "twoDir", "twoDirA1", "twoFileA1.txt"),
					"content":  "Sample content [Trilio] [DataBackupRestore]",
					"acl":      "u:2004:rw,g:9004:rw",
					"xattr":    "name twoFileA1.txt",
				},
				{
					"action":   "delete",
					"fullPath": path.Join(dst, ".hiddenFile"),
					"content":  "Hidden file/dir name starts with .",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "twoDir", "twoDirA1", "supernew"),
					"content":  "Sample new content",
					"xattr":    "I am super new file",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "zDir", "zDirFile"),
					"content":  "I am the zDirFile. I am the update in zDirFile",
				},
				{
					"action":   "create",
					"fullPath": path.Join(dst, "My file for test"),
					"content":  "File with spaces in the name",
				},
			},
			"Links": []map[string]string{
				{
					"action":   "delete",
					"target":   path.Join("twoDir", "twoDirA1"),
					"fullPath": path.Join(dst, "lnDir"),
				},
				{
					"action":   "create",
					"target":   path.Join("ondDirA1", "oneFileA1.txt"),
					"fullPath": path.Join(dst, "oneDir", "oneDirLn"),
				},
				{
					"action":   "create",
					"target":   path.Join("twoDir", "twoDirA1", "supernew"),
					"fullPath": path.Join(dst, "lnsuper"),
				},
				{
					"action":   "create",
					"target":   path.Join("oneDir", "oneDirLn"),
					"fullPath": path.Join(dst, "topDirOneLn"),
				},
				{
					"action":   "create",
					"target":   path.Join("zDirFile"),
					"fullPath": path.Join(dst, "zDir", "zDirLn"),
				},
				{
					"action":   "create",
					"target":   path.Join("zDir", "zDirLn"),
					"fullPath": path.Join(dst, "topDirZLn"),
				},
			},
		},
	}

	// In case of Full, the content to be created and content to be
	// validated is same that's why returning same
	if backupType == "Full" {
		return createFSContent[backupType].(map[string]interface{}),
			createFSContent[backupType].(map[string]interface{})
	}

	// In case of Incremental, the content to be created is limited
	// but the validation needs full content
	return createFSContent[backupType].(map[string]interface{}),
		validateFSContent[backupType].(map[string]interface{})
}

func setXAttrAndACL(src map[string]string) error {
	if val, ok := src["xattr"]; ok {
		setErr := unix.Setxattr(src["fullPath"], "user.comment", []byte(val), 0)
		if setErr != nil {
			return setErr
		}
	}
	// Set default ACL
	if dVal, dOk := src["dacl"]; dOk {
		cmd := fmt.Sprintf("/usr/bin/setfacl -d -m %s %s", dVal, src["fullPath"])
		_, cmdErr := shell.RunCmd(cmd)
		if cmdErr != nil {
			log.Errorf("Command:[%s] failed with err: %s", cmd, cmdErr)
			return cmdErr
		}
	}
	// Set access ACL
	if val, ok := src["acl"]; ok {
		cmd := fmt.Sprintf("/usr/bin/setfacl -m %s %s", val, src["fullPath"])
		_, cmdErr := shell.RunCmd(cmd)
		if cmdErr != nil {
			log.Errorf("Command:[%s] failed with err: %s", cmd, cmdErr)
			return cmdErr
		}
	}

	return nil
}

func validateXAttrAndACL(src map[string]string, isDir bool) error {
	var (
		aclStr  string
		reqAcls []string
	)
	if val, ok := src["xattr"]; ok {
		bytesXAttr, getErr := xattr.Get(src["fullPath"], "user.comment")
		if getErr != nil {
			return getErr
		}
		xAttr := string(bytesXAttr)
		if xAttr != val {
			return fmt.Errorf("failed validation for path %s. Expected XAttr value is \"%s\" but "+
				"actual value is \"%s\"", src["fullPath"], val, xAttr)
		}
	}

	if val, ok := src["dacl"]; ok {
		acls := strings.Split(val, ",")
		reqAcls = append(reqAcls, acls...)
	}
	if val, ok := src["acl"]; ok {
		acls := strings.Split(val, ",")
		reqAcls = append(reqAcls, acls...)
	}

	if len(reqAcls) > 0 {
		aACL, aACLErr := acl.Get(src["fullPath"])
		if aACLErr != nil {
			return aACLErr
		}
		aclStr = aACL.String()
		if isDir {
			dACL, dACLErr := acl.GetDefault(src["fullPath"])
			if dACLErr != nil {
				return dACLErr
			}
			aclStr = aclStr + "," + dACL.String()
		}
		for _, a := range reqAcls {
			if !strings.Contains(aclStr, a) {
				return fmt.Errorf("failed validation for path %s. ACL %s is not present "+
					"in %+v", src["fullPath"], a, aclStr)
			}
		}
	}

	return nil
}

// CreateImageFromDirectory Creates the complete backup qcow2 image for the source path passed
// params:
// input=>  format:dest image format.
// 			srcPath:path to create qcow2.
// 			destPath:path to store qcow2.
// output=> stdOut: stderr if command execution fails else stdout.
// 		    err: non-nil error if command execution failed.
func CreateImageFromDirectory(format, srcPath, destPath string) (string, error) {
	cmd := fmt.Sprintf("virt-make-fs -v --format=%s %s %s", format, srcPath, destPath)
	log.Debugf("Create qcow2 command from filesystem, command: %s, source path: %s, destination path: %s", cmd, srcPath, destPath)

	err := shell.RunCmdWithOutput(cmd)
	if err != nil {
		log.Errorf("Command:[%s] => (FAILED), err: %+v, with stderror: %s, ", cmd, err, err.Error())
		return err.Error(), err
	}

	log.Debugf("Command:[%s] => (Success)", cmd)
	return "", nil
}
