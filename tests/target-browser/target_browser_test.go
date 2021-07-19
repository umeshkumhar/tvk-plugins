package targetbrowsertest

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"sync"
	"time"

	guid "github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"
	"github.com/trilioData/tvk-plugins/internal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/trilioData/tvk-plugins/cmd/target-browser/cmd"
	"github.com/trilioData/tvk-plugins/internal/utils/shell"
)

type backupPlan struct {
	BackupPlanName            string      `json:"Name"`
	BackupPlanUID             string      `json:"UID"`
	BackupPlanType            string      `json:"Type"`
	InstanceID                string      `json:"TVK Instance"`
	SuccessfulBackup          int         `json:"Successful Backup"`
	SuccessfulBackupTimestamp metav1.Time `json:"Timestamp"`
}
type backup struct {
	Name           string      `json:"Name"`
	UID            string      `json:"UID"`
	Type           string      `json:"Type"`
	Size           string      `json:"Size"`
	Status         string      `json:"Status"`
	BackupPlanUID  string      `json:"BackupPlan UID"`
	CreationTime   metav1.Time `json:"Start Time"`
	CompletionTime metav1.Time `json:"End Time"`
}

const (
	BackupPlanType         = "backupPlanType"
	Name                   = "name"
	SuccessfulBackups      = "successfulBackups"
	BackupTimestamp        = "backupTimestamp"
	cmdBackupPlan          = cmd.BackupPlanCmdName
	cmdBackup              = cmd.BackupCmdName
	cmdMetadata            = cmd.MetadataCmdName
	cmdGet                 = "get"
	flagPrefix             = "--"
	flagOrderBy            = flagPrefix + cmd.OrderByFlag
	flagTvkInstanceUIDFlag = flagPrefix + cmd.TvkInstanceUIDFlag
	flagBackupUIDFlag      = flagPrefix + cmd.BackupUIDFlag
	flagBackupStatus       = flagPrefix + cmd.BackupStatusFlag
	flagBackupPlanUIDFlag  = flagPrefix + cmd.BackupPlanUIDFlag
	flagPageSize           = flagPrefix + cmd.PageSizeFlag
	flagTargetNamespace    = flagPrefix + cmd.TargetNamespaceFlag
	flagTargetName         = flagPrefix + cmd.TargetNameFlag
	flagKubeConfig         = flagPrefix + cmd.KubeConfigFlag
	flagCaCert             = flagPrefix + cmd.CertificateAuthorityFlag
	flagInsecureSkip       = flagPrefix + cmd.InsecureSkipTLSFlag
	flagUseHTTPS           = flagPrefix + cmd.UseHTTPS
	flagOutputFormat       = flagPrefix + cmd.OutputFormatFlag
)

var (
	kubeConf, _         = internal.NewConfigFromCommandline("")
	targetBrowserStatus = []string{"Available", "Failed", "InProgress"}
	commonArgs          = []string{flagTargetName, TargetName, flagTargetNamespace,
		installNs, flagOutputFormat, internal.FormatJSON, flagKubeConfig, kubeConf}
)

var _ = Describe("Target Browser Tests", func() {

	Context("Global Flag's test-cases", func() {

		Context("test-cases without target creation", func() {

			It(fmt.Sprintf("Should consider default value if flag %s not given", flagKubeConfig), func() {
				args := []string{cmdGet, cmdBackupPlan}
				testArgs := []string{flagTargetName, TargetName, flagTargetNamespace, installNs}
				args = append(args, testArgs...)
				command := exec.Command(targetBrowserBinaryFilePath, args...)
				output, err := command.CombinedOutput()
				Expect(err).Should(HaveOccurred())
				Expect(string(output)).Should(ContainSubstring(".kube/config: no such file or directory"))
			})

			It(fmt.Sprintf("Should consider KUBECONFIG env variable's value if flag %s given with zero value", flagKubeConfig), func() {
				args := []string{cmdGet, cmdBackupPlan}
				testArgs := []string{flagKubeConfig, "", flagTargetName, TargetName, flagTargetNamespace, installNs}
				args = append(args, testArgs...)
				command := exec.Command(targetBrowserBinaryFilePath, args...)
				output, err := command.CombinedOutput()
				Expect(err).Should(HaveOccurred())
				Expect(string(output)).Should(ContainSubstring(fmt.Sprintf("targets.triliovault.trilio.io \"%s\" not found", TargetName)))
			})

			It(fmt.Sprintf("Should fail if flag %s not given", flagTargetName), func() {
				args := []string{cmdGet, cmdBackupPlan}
				testArgs := []string{flagTargetNamespace, installNs, flagKubeConfig, kubeConf}
				args = append(args, testArgs...)
				command := exec.Command(targetBrowserBinaryFilePath, args...)
				output, err := command.CombinedOutput()
				Expect(err).Should(HaveOccurred())
				Expect(string(output)).Should(ContainSubstring(fmt.Sprintf("[%s] flag value cannot be empty", cmd.TargetNameFlag)))
			})

			It(fmt.Sprintf("Should fail if flag %s is given with zero value", flagTargetName), func() {
				args := []string{cmdGet, cmdBackupPlan}
				testArgs := []string{flagTargetName, "", flagTargetNamespace, installNs, flagKubeConfig, kubeConf}
				args = append(args, testArgs...)
				command := exec.Command(targetBrowserBinaryFilePath, args...)
				output, err := command.CombinedOutput()
				Expect(err).Should(HaveOccurred())
				Expect(string(output)).Should(ContainSubstring(fmt.Sprintf("[%s] flag value cannot be empty", cmd.TargetNameFlag)))
			})

			It(fmt.Sprintf("Should fail if flag %s & %s both are given", flagCaCert, flagInsecureSkip), func() {
				args := []string{cmdGet, cmdBackupPlan, flagInsecureSkip, strconv.FormatBool(true), flagCaCert, "ca-cert-file-path"}
				args = append(args, commonArgs...)
				command := exec.Command(targetBrowserBinaryFilePath, args...)
				output, err := command.CombinedOutput()
				Expect(err).Should(HaveOccurred())
				Expect(string(output)).Should(ContainSubstring(fmt.Sprintf("[%s] flag cannot be provided if [%s] is provided",
					cmd.InsecureSkipTLSFlag, cmd.CertificateAuthorityFlag)))
			})

			It(fmt.Sprintf("Should fail if flag %s is given with zero value", flagCaCert), func() {
				args := []string{cmdGet, cmdBackupPlan, flagCaCert, ""}
				args = append(args, commonArgs...)
				command := exec.Command(targetBrowserBinaryFilePath, args...)
				output, err := command.CombinedOutput()
				Expect(err).Should(HaveOccurred())
				Expect(string(output)).Should(ContainSubstring(fmt.Sprintf("[%s] flag value cannot be empty", cmd.CertificateAuthorityFlag)))
			})
		})

		Context("test-cases with target 'browsingEnabled=false'", func() {
			var (
				once   sync.Once
				isLast bool
			)

			BeforeEach(func() {
				once.Do(func() {
					createTarget(false)
				})
			})

			AfterEach(func() {
				if isLast {
					deleteTarget()
				}
			})

			It(fmt.Sprintf("Should fail if flag %s is given with incorrect value", flagTargetName), func() {
				incorrectTarget := "incorrect-target-name"
				args := []string{cmdGet, cmdBackupPlan, flagKubeConfig, kubeConf}
				testArgs := []string{flagTargetName, incorrectTarget, flagTargetNamespace, installNs}
				args = append(args, testArgs...)
				command := exec.Command(targetBrowserBinaryFilePath, args...)
				output, err := command.CombinedOutput()
				Expect(err).Should(HaveOccurred())
				Expect(string(output)).Should(ContainSubstring(fmt.Sprintf("targets.triliovault.trilio.io \"%s\" not found", incorrectTarget)))
			})

			It("Should fail if target CR status does not have 'browsingEnabled=true'", func() {
				args := []string{cmdGet, cmdBackupPlan}
				args = append(args, commonArgs...)
				cmd := exec.Command(targetBrowserBinaryFilePath, args...)
				output, err := cmd.CombinedOutput()
				Expect(err).Should(HaveOccurred())
				Expect(string(output)).Should(ContainSubstring(fmt.Sprintf("browsing is not enabled for "+
					"given target %s namespace %s", TargetName, installNs)))
			})

			It(fmt.Sprintf("Should consider default value if flag %s is not given", flagTargetNamespace), func() {
				isLast = true
				args := []string{cmdGet, cmdBackupPlan, flagKubeConfig, kubeConf}
				testArgs := []string{flagTargetName, TargetName}
				args = append(args, testArgs...)
				command := exec.Command(targetBrowserBinaryFilePath, args...)
				output, err := command.CombinedOutput()
				Expect(err).Should(HaveOccurred())
				Expect(string(output)).Should(ContainSubstring(fmt.Sprintf("targets.triliovault.trilio.io \"%s\" not found", TargetName)))
			})
		})

		Context("test-cases with target 'browsingEnabled=true'", func() {
			BeforeEach(func() {
				createTarget(true)
				deleteTargetBrowserIngress()
			})

			AfterEach(func() {
				deleteTarget()
			})

			It("Should fail if target browser's ingress resource not found", func() {
				args := []string{cmdGet, cmdBackupPlan}
				args = append(args, commonArgs...)
				command := exec.Command(targetBrowserBinaryFilePath, args...)
				output, err := command.CombinedOutput()
				Expect(err).Should(HaveOccurred())
				Expect(string(output)).Should(ContainSubstring(fmt.Sprintf("either tvkHost or targetBrowserPath could not"+
					" retrieved for target %s namespace %s", TargetName, installNs)))
			})

		})

		Context("test-cases with target 'browsingEnabled=true' && TVK host is serving HTTPS", func() {
			var (
				once   sync.Once
				isLast bool
			)

			BeforeEach(func() {
				once.Do(func() {
					switchTvkHostFromHTTPToHTTPS()
					time.Sleep(time.Second * 10)
					createTarget(true)
				})
			})

			AfterEach(func() {
				if isLast {
					deleteTarget()
					switchTvkHostFromHTTPSToHTTP()
					time.Sleep(time.Second * 10)
				}
			})

			It(fmt.Sprintf("Should pass if flag %s & %s both are provided", cmd.UseHTTPS, cmd.CertificateAuthorityFlag), func() {
				args := []string{cmdGet, cmdBackupPlan, flagUseHTTPS, flagCaCert, filepath.Join(testDataDirRelPath, tlsCertFile)}
				args = append(args, commonArgs...)
				command := exec.Command(targetBrowserBinaryFilePath, args...)
				_, err := command.CombinedOutput()
				Expect(err).ShouldNot(HaveOccurred())
			})

			It(fmt.Sprintf("Should fail if flag %s is not provided", cmd.UseHTTPS), func() {
				args := []string{cmdGet, cmdBackupPlan}
				args = append(args, commonArgs...)
				command := exec.Command(targetBrowserBinaryFilePath, args...)
				output, err := command.CombinedOutput()
				Expect(err).Should(HaveOccurred())
				Expect(string(output)).Should(ContainSubstring("Client.Timeout exceeded while awaiting headers"))
			})

			It(fmt.Sprintf("Should fail if flag %s is provided but %s is not provided", cmd.UseHTTPS, cmd.CertificateAuthorityFlag), func() {
				args := []string{cmdGet, cmdBackupPlan, flagUseHTTPS}
				args = append(args, commonArgs...)
				command := exec.Command(targetBrowserBinaryFilePath, args...)
				output, err := command.CombinedOutput()
				Expect(err).Should(HaveOccurred())
				Expect(string(output)).Should(ContainSubstring("certificate signed by unknown authority"))
			})

			It(fmt.Sprintf("Should fail if flag %s is provided but %s is not provided", cmd.CertificateAuthorityFlag, cmd.UseHTTPS), func() {
				isLast = true
				args := []string{cmdGet, cmdBackupPlan, flagCaCert, filepath.Join(testDataDirRelPath, tlsCertFile)}
				args = append(args, commonArgs...)
				command := exec.Command(targetBrowserBinaryFilePath, args...)
				output, err := command.CombinedOutput()
				Expect(err).Should(HaveOccurred())
				Expect(string(output)).Should(ContainSubstring("Client.Timeout exceeded while awaiting headers"))
			})
		})
	})

	Context("Sorting and filtering Operations on multiple columns of backupPlan & backup", func() {
		var (
			backupPlanUIDs          []string
			noOfBackupPlansToCreate = 8
			noOfBackupsToCreate     = 2
			once                    sync.Once
			isLast                  bool
			backupUID               string
		)
		BeforeEach(func() {
			once.Do(func() {
				// being used by target - only once Before all It in this context
				// create target with browsing enabled & create all files & directories required for this Context in NFS server
				// once.Do run once for this Context
				backupUID = guid.New().String()
				createTarget(true)
				output, _ := exec.Command(createBackupScript, strconv.Itoa(noOfBackupPlansToCreate),
					strconv.Itoa(noOfBackupsToCreate), "true", backupUID).Output()
				log.Info("Shell Script Output: ", string(output))
				backupPlanUIDs = verifyBackupPlansAndBackupsOnNFS(noOfBackupPlansToCreate, noOfBackupPlansToCreate*noOfBackupsToCreate)
				time.Sleep(2 * time.Minute) //wait to sync data on target browser
			})

		})

		AfterEach(func() {
			if isLast {
				//delete target & remove all files & directories created for this Context - only once After all It in this context
				deleteTarget()
				for _, backupPlans := range backupPlanUIDs {
					_, err := shell.RmRf(filepath.Join(TargetLocation, backupPlans))
					Expect(err).To(BeNil())
				}
			}
		})

		It(fmt.Sprintf("Should fail if flag %s is given with zero value", flagOrderBy), func() {
			args := []string{cmdGet, cmdBackupPlan}
			args = append(args, commonArgs...)
			args = append(args, flagOrderBy)
			command := exec.Command(targetBrowserBinaryFilePath, args...)
			output, err := command.CombinedOutput()
			Expect(err).Should(HaveOccurred())
			Expect(string(output)).Should(ContainSubstring(fmt.Sprintf("flag needs an argument: %s", flagOrderBy)))
		})

		It("Should sort backupPlans on BackupPlan Application Type in ascending order", func() {
			args := []string{cmdGet, cmdBackupPlan, flagOrderBy, BackupPlanType}
			backupPlanData := runCmdBackupPlan(args)
			for index := 0; index < len(backupPlanData)-1; index++ {
				Expect(backupPlanData[index].BackupPlanType <= backupPlanData[index+1].BackupPlanType).Should(BeTrue())
			}
		})

		It("Should sort backupPlans on BackupPlan Application Type in descending order", func() {
			args := []string{cmdGet, cmdBackupPlan, flagOrderBy, "-" + BackupPlanType}
			backupPlanData := runCmdBackupPlan(args)

			for index := 0; index < len(backupPlanData)-1; index++ {
				Expect(backupPlanData[index].BackupPlanType >= backupPlanData[index+1].BackupPlanType).Should(BeTrue())
			}
		})
		It("Should sort backupPlans on BackupPlan Name in ascending order", func() {
			args := []string{cmdGet, cmdBackupPlan, flagOrderBy, Name}
			backupPlanData := runCmdBackupPlan(args)

			for index := 0; index < len(backupPlanData)-1; index++ {
				Expect(backupPlanData[index].BackupPlanName <= backupPlanData[index+1].BackupPlanName).Should(BeTrue())
			}
		})

		It("Should sort backupPlans on BackupPlan Name in ascending order", func() {
			args := []string{cmdGet, cmdBackupPlan, flagOrderBy, "-" + Name}
			backupPlanData := runCmdBackupPlan(args)

			for index := 0; index < len(backupPlanData)-1; index++ {
				Expect(backupPlanData[index].BackupPlanName >= backupPlanData[index+1].BackupPlanName).Should(BeTrue())
			}
		})
		It("Should sort backupPlans on Successful Backup Count in ascending order", func() {
			args := []string{cmdGet, cmdBackupPlan, flagOrderBy, SuccessfulBackups}
			backupPlanData := runCmdBackupPlan(args)

			for index := 0; index < len(backupPlanData)-1; index++ {
				Expect(backupPlanData[index].SuccessfulBackup <= backupPlanData[index+1].SuccessfulBackup).Should(BeTrue())
			}
		})
		It("Should sort backupPlans on Successful Backup Count in descending order", func() {
			args := []string{cmdGet, cmdBackupPlan, flagOrderBy, "-" + SuccessfulBackups}
			backupPlanData := runCmdBackupPlan(args)
			for index := 0; index < len(backupPlanData)-1; index++ {
				Expect(backupPlanData[index].SuccessfulBackup >= backupPlanData[index+1].SuccessfulBackup).Should(BeTrue())
			}
		})
		It("Should sort backupPlans on LastSuccessfulBackupTimestamp in ascending order", func() {
			args := []string{cmdGet, cmdBackupPlan, flagOrderBy, BackupTimestamp}
			backupPlanData := runCmdBackupPlan(args)

			for index := 0; index < len(backupPlanData)-1; index++ {
				if backupPlanData[index].SuccessfulBackup != 0 && backupPlanData[index+1].SuccessfulBackup != 0 &&
					!backupPlanData[index].SuccessfulBackupTimestamp.Time.Equal(backupPlanData[index+1].
						SuccessfulBackupTimestamp.Time) {
					Expect(backupPlanData[index].SuccessfulBackupTimestamp.Time.Before(backupPlanData[index+1].
						SuccessfulBackupTimestamp.Time)).Should(BeTrue())
				}
			}
		})
		It("Should sort backupPlans on LastSuccessfulBackupTimestamp in descending order", func() {
			args := []string{cmdGet, cmdBackupPlan, flagOrderBy, "-" + BackupTimestamp}
			backupPlanData := runCmdBackupPlan(args)

			for index := 0; index < len(backupPlanData)-1; index++ {
				if backupPlanData[index].SuccessfulBackup != 0 && backupPlanData[index+1].SuccessfulBackup != 0 &&
					!backupPlanData[index].SuccessfulBackupTimestamp.Time.Equal(backupPlanData[index+1].
						SuccessfulBackupTimestamp.Time) {
					Expect(backupPlanData[index].SuccessfulBackupTimestamp.Time.After(backupPlanData[index+1].
						SuccessfulBackupTimestamp.Time)).Should(BeTrue())
				}
			}
		})
		It("Should sort backups on name in ascending order", func() {
			args := []string{cmdGet, cmdBackup, flagBackupPlanUIDFlag, backupPlanUIDs[0], flagOrderBy, Name}
			backupData := runCmdBackup(args)

			for index := 0; index < len(backupData)-1; index++ {
				Expect(backupData[index].Name <= backupData[index+1].Name).Should(BeTrue())
			}
		})
		It("Should sort backups on name in descending order", func() {
			args := []string{cmdGet, cmdBackup, flagBackupPlanUIDFlag, backupPlanUIDs[0], flagOrderBy, "-" + Name}
			backupData := runCmdBackup(args)

			for index := 0; index < len(backupData)-1; index++ {
				Expect(backupData[index].Name >= backupData[index+1].Name).Should(BeTrue())
			}
		})

		It(fmt.Sprintf("Should fail if flag %s is given with zero value", flagBackupStatus), func() {
			args := []string{cmdGet, cmdBackup, flagBackupPlanUIDFlag, backupPlanUIDs[1], flagBackupStatus}
			args = append(args, commonArgs...)
			args = append(args, flagBackupStatus)
			command := exec.Command(targetBrowserBinaryFilePath, args...)
			output, err := command.CombinedOutput()
			Expect(err).Should(HaveOccurred())
			Expect(string(output)).Should(ContainSubstring(fmt.Sprintf("flag needs an argument: %s", flagBackupStatus)))
		})

		It("Should filter backupPlans on BackupPlan Application Type", func() {
			// select random backup status for status filter
			statusFilterValue := targetBrowserStatus[rand.Intn(len(targetBrowserStatus))]
			args := []string{cmdGet, cmdBackup, flagBackupPlanUIDFlag, backupPlanUIDs[1], flagBackupStatus, statusFilterValue}
			backupData := runCmdBackup(args)
			// compare backups status with status value passed as arg
			for index := 0; index < len(backupData); index++ {
				Expect(backupData[index].Status).Should(Equal(statusFilterValue))
			}
		})

		It("Should get one page backupPlan", func() {
			args := []string{cmdGet, cmdBackupPlan, flagPageSize, "1"}
			backupPlanData := runCmdBackupPlan(args)
			Expect(len(backupPlanData)).To(Equal(1))
		})

		It("Should get one page backup", func() {
			args := []string{cmdGet, cmdBackup, flagBackupPlanUIDFlag, backupPlanUIDs[1], flagPageSize, "1"}
			backupData := runCmdBackup(args)
			Expect(len(backupData)).To(Equal(1))
		})

		It("Should get one backupPlan for specific backupPlan UID", func() {
			args := []string{cmdGet, cmdBackupPlan, backupPlanUIDs[1]}
			backupPlanData := runCmdBackupPlan(args)
			Expect(len(backupPlanData)).To(Equal(1))
			Expect(backupPlanData[0].BackupPlanUID).To(Equal(backupPlanUIDs[1]))
		})

		It("Should get two backupPlan for backupPlan UIDs", func() {
			args := []string{cmdGet, cmdBackupPlan, backupPlanUIDs[0], backupPlanUIDs[1]}
			backupPlanData := runCmdBackupPlan(args)
			Expect(len(backupPlanData)).To(Equal(2))
			Expect(backupPlanData[0].BackupPlanUID).To(Equal(backupPlanUIDs[0]))
			Expect(backupPlanData[1].BackupPlanUID).To(Equal(backupPlanUIDs[1]))
		})

		It("Should get one backup for specific backup UID", func() {
			args := []string{cmdGet, cmdBackup, backupUID}
			backupData := runCmdBackup(args)
			Expect(len(backupData)).To(Equal(1))
			Expect(backupData[0].UID).To(Equal(backupUID))
		})

		It("Should fail for invalid backupPlan UID", func() {
			args := []string{cmdGet, cmdBackupPlan, "invalidUID"}
			args = append(args, commonArgs...)
			command := exec.Command(targetBrowserBinaryFilePath, args...)
			output, err := command.CombinedOutput()
			Expect(err).Should(HaveOccurred())
			Expect(string(output)).Should(ContainSubstring("404 Not Found"))
		})

		It("Should get two backup for backup UID", func() {
			args := []string{cmdGet, cmdBackup, backupUID, backupUID}
			backupData := runCmdBackup(args)
			Expect(len(backupData)).To(Equal(2))
			Expect(backupData[0].UID).To(Equal(backupUID))
		})

		It("Should get one page backup without backupPlan UID", func() {
			args := []string{cmdGet, cmdBackup, backupUID, flagPageSize, "1"}
			backupData := runCmdBackup(args)
			Expect(len(backupData)).To(Equal(1))
			Expect(backupData[0].UID).To(Equal(backupUID))
		})

		It("Should get zero backup", func() {
			args := []string{cmdGet, cmdBackup, flagBackupPlanUIDFlag, "invalid"}
			backupData := runCmdBackup(args)
			Expect(len(backupData)).Should(Equal(0))
		})

		It(fmt.Sprintf("Should fail if flag %s is given with zero value", flagBackupPlanUIDFlag), func() {
			isLast = true
			args := []string{cmdGet, cmdBackup}
			args = append(args, commonArgs...)
			args = append(args, flagBackupPlanUIDFlag)
			command := exec.Command(targetBrowserBinaryFilePath, args...)
			output, err := command.CombinedOutput()
			Expect(err).Should(HaveOccurred())
			Expect(string(output)).Should(ContainSubstring(fmt.Sprintf("flag needs an argument: %s", flagBackupPlanUIDFlag)))

		})
	})

	Context("Filtering BackupPlans based on TVK Instance ID", func() {

		var (
			backupPlanUIDs      []string
			tvkInstanceIDValues []string
			once                sync.Once
			isLast              bool
		)
		BeforeEach(func() {
			once.Do(func() {
				createTarget(true)
				// Generating backupPlans and backups with different TVK instance UID
				tvkInstanceIDValues = []string{
					guid.New().String(),
					guid.New().String(),
				}
				for _, value := range tvkInstanceIDValues {
					_, err := exec.Command(createBackupScript, "1", "1", "mutate-tvk-id", value).Output()
					Expect(err).To(BeNil())
				}
				time.Sleep(1 * time.Minute) //wait to sync data on target browser
				backupPlanUIDs = verifyBackupPlansAndBackupsOnNFS(2, 2)
			})
		})
		AfterEach(func() {
			if isLast {
				deleteTarget()
				for _, backupPlans := range backupPlanUIDs {
					_, err := shell.RmRf(filepath.Join(TargetLocation, backupPlans))
					Expect(err).To(BeNil())
				}
			}
		})

		It("Should filter backupPlans on TVK Instance UID", func() {
			for _, value := range tvkInstanceIDValues {
				args := []string{cmdGet, cmdBackupPlan, flagTvkInstanceUIDFlag, value}
				backupPlanData := runCmdBackupPlan(args)
				Expect(len(backupPlanData)).To(Equal(1))
				Expect(backupPlanData[0].InstanceID).Should(Equal(value))
			}
		})

		It("Should get zero result backupPlans on TVK Instance UID", func() {
			for _, value := range tvkInstanceIDValues {
				value += "s"
				args := []string{cmdGet, cmdBackupPlan, flagTvkInstanceUIDFlag, value}
				backupPlanData := runCmdBackupPlan(args)
				Expect(len(backupPlanData)).To(Equal(0))
			}
		})

		It(fmt.Sprintf("Should fail if flag %s is given with zero value", flagTvkInstanceUIDFlag), func() {
			args := []string{cmdGet, cmdBackupPlan}
			args = append(args, commonArgs...)
			args = append(args, flagTvkInstanceUIDFlag)
			command := exec.Command(targetBrowserBinaryFilePath, args...)
			output, err := command.CombinedOutput()
			Expect(err).Should(HaveOccurred())
			Expect(string(output)).Should(ContainSubstring(fmt.Sprintf("flag needs an argument: %s", flagTvkInstanceUIDFlag)))
		})

		It("Should filter backupPlans on TVK Instance UID and order by backupPlan name ascending order", func() {
			isLast = true
			for _, value := range tvkInstanceIDValues {
				args := []string{cmdGet, cmdBackupPlan, flagTvkInstanceUIDFlag, value, flagOrderBy, Name}
				backupPlanData := runCmdBackupPlan(args)
				Expect(len(backupPlanData)).To(Equal(1))
				Expect(backupPlanData[0].InstanceID).Should(Equal(value))
				for index := 0; index < len(backupPlanData)-1; index++ {
					Expect(backupPlanData[index].BackupPlanName <= backupPlanData[index+1].BackupPlanName).Should(BeTrue())
				}
			}
		})
	})

	Context("Metadata filtering Operations on different fields of backupPlan & backup", func() {
		var (
			backupPlanUIDs          []string
			noOfBackupPlansToCreate = 1
			noOfBackupsToCreate     = 1
			once                    sync.Once
			isLast                  bool
			backupUID               string
		)

		BeforeEach(func() {
			once.Do(func() {
				backupUID = guid.New().String()
				createTarget(true)
				output, _ := exec.Command(createBackupScript, strconv.Itoa(noOfBackupPlansToCreate),
					strconv.Itoa(noOfBackupsToCreate), "all_type_backup", backupUID).Output()

				log.Info("Shell Script Output: ", string(output))
				backupPlanUIDs = verifyBackupPlansAndBackupsOnNFS(noOfBackupPlansToCreate, noOfBackupPlansToCreate*noOfBackupsToCreate)
				time.Sleep(1 * time.Minute) //wait to sync data on target browser
			})
		})

		AfterEach(func() {
			if isLast {
				// delete target & remove all files & directories created for this Context - only once After all It in this context
				deleteTarget()
				for _, backupPlans := range backupPlanUIDs {
					_, err := shell.RmRf(filepath.Join(TargetLocation, backupPlans))
					Expect(err).To(BeNil())
				}
			}
		})
		It(fmt.Sprintf("Should fail if flag %s not given", cmd.BackupPlanUIDFlag), func() {
			args := []string{cmdGet, cmdMetadata}
			args = append(args, commonArgs...)
			command := exec.Command(targetBrowserBinaryFilePath, args...)
			output, err := command.CombinedOutput()
			Expect(err).Should(HaveOccurred())
			Expect(string(output)).Should(ContainSubstring(fmt.Sprintf("required flag(s) \"%s\", \"%s\" not set",
				cmd.BackupPlanUIDFlag, cmd.BackupUIDFlag)))
		})

		It(fmt.Sprintf("Should fail if flag %s not given", cmd.BackupUIDFlag), func() {
			args := []string{cmdGet, cmdMetadata}
			args = append(args, commonArgs...)
			command := exec.Command(targetBrowserBinaryFilePath, args...)
			output, err := command.CombinedOutput()
			Expect(err).Should(HaveOccurred())
			Expect(string(output)).Should(ContainSubstring(fmt.Sprintf("required flag(s) \"%s\", \"%s\" not set",
				cmd.BackupPlanUIDFlag, cmd.BackupUIDFlag)))
		})

		It(fmt.Sprintf("Should fail if flag %s is given with zero value", flagBackupPlanUIDFlag), func() {
			args := []string{cmdGet, cmdMetadata}
			args = append(args, commonArgs...)
			args = append(args, flagBackupPlanUIDFlag)
			command := exec.Command(targetBrowserBinaryFilePath, args...)
			output, err := command.CombinedOutput()
			Expect(err).Should(HaveOccurred())
			Expect(string(output)).Should(ContainSubstring(fmt.Sprintf("flag needs an argument: %s", flagBackupPlanUIDFlag)))
		})

		It("Should filter metadata on BackupPlan and backup", func() {
			isLast = true
			backupPlanUID := backupPlanUIDs[0]
			args := []string{cmdGet, cmdMetadata, flagBackupPlanUIDFlag, backupPlanUID, flagBackupUIDFlag, backupUID}
			args = append(args, commonArgs...)
			command := exec.Command(targetBrowserBinaryFilePath, args...)
			output, err := command.CombinedOutput()
			if err != nil {
				Fail(fmt.Sprintf("Error to execute command %s", err.Error()))
			}
			log.Infof("Metadata is %s", output)
			re := regexp.MustCompile("(?m)[\r\n]+^.*location.*$")
			metadata := re.ReplaceAllString(string(output), "")
			jsonFile, err := os.Open(filepath.Join(testDataDirRelPath, "backup-metadata.json"))
			Expect(err).To(BeNil())

			defer jsonFile.Close()

			expectedMetadata, _ := ioutil.ReadAll(jsonFile)
			Expect(reflect.DeepEqual(expectedMetadata, metadata))
		})
	})
})
