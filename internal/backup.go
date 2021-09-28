package internal

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	log "github.com/sirupsen/logrus"
)

// Backup contains the configuration required for doing a backup
type Backup struct {
	HourlyBackups  int
	DailyBackups   int
	WeeklyBackups  int
	MonthlyBackups int
	S3Bucket       string
	S3Path         string
	DataDirectory  string
	AwsSession     *session.Session
	S3Service      s3iface.S3API
}

// Run performs a backup
func (b Backup) Run(backupType string) {
	log.Info("Beginning backup")
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	log.Info("Compressing directory")
	archivePath, err := b.compressDirectory(now, backupType)

	if err != nil {
		log.Errorf("failed to compress directory: error = %s", err)
		return
	}

	log.Info("Uploading to S3")
	if err = b.uploadToS3(now, backupType, archivePath); err != nil {
		log.Errorf("failed to upload to S3: error = %s", err)
		return
	}

	log.Info("Pruning old backups from S3")
	if err = b.pruneS3(backupType); err != nil {
		// report but carry on anyway
		log.Errorf("failed to prune old backups from S3: error = %s", err)
	}

	log.Info("Removing temporary backup directory")
	if err = b.removeBackupDirectory(); err != nil {
		log.Warnf("failed to delete backup directory: error = %s", err)
	}

	log.Info("Backup complete")
}

func (b Backup) compressDirectory(now string, backupType string) (string, error) {
	backupFile, err := os.OpenFile(b.DataDirectory+"/BACKUP_DATE", os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return "", err
	}

	_, err = backupFile.WriteString(fmt.Sprintf("%s/%s\n", backupType, now))
	if err != nil {
		return "", err
	}

	tempDir := os.TempDir() + "/backups"
	err = os.Mkdir(tempDir, 0700)
	if err != nil {
		return "", err
	}
	file, err := ioutil.TempFile(tempDir, "backup*.tar.gz")
	if err != nil {
		return "", err
	}
	defer func(file *os.File) {
		if err := file.Close(); err != nil {
			log.Warnf("failed to close of backup temp file: error = %s", err)
		}
	}(file)

	gw := gzip.NewWriter(file)
	defer func(gw *gzip.Writer) {
		if err := gw.Close(); err != nil {
			log.Warnf("failed to close gzip writer for backup: %s", err)
		}
	}(gw)

	tw := tar.NewWriter(gw)
	defer func(tw *tar.Writer) {
		if err := tw.Close(); err != nil {
			log.Warnf("failed to close tar writer for backup: %s", err)
		}
	}(tw)

	err = filepath.Walk(
		b.DataDirectory,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			afErr := b.addFile(tw, path)
			if afErr != nil {
				return afErr
			}

			return nil
		},
	)

	if err != nil {
		return "", err
	}

	log.Infof("Archive created successfully: output file = %s", file.Name())
	return file.Name(), nil
}

func (b Backup) uploadToS3(now string, backupType string, path string) error {
	uploader := s3manager.NewUploader(b.AwsSession)
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		if err := file.Close(); err != nil {
			log.Warnf("failed to close S3 upload file: file = %s, error = %s", path, err)
		}
	}(file)

	uploadPath := fmt.Sprintf("%s/%s/%s.tar.gz", b.S3Path, backupType, now)

	log.Infof("Uploading to backup file: key = %s", uploadPath)

	uploadInput := s3manager.UploadInput{
		Body:   file,
		Bucket: aws.String(b.S3Bucket),
		Key:    aws.String(uploadPath),
	}

	_, err = uploader.Upload(&uploadInput)
	if err != nil {
		return err
	}

	log.Info("Uploaded backup successfully: file = %s, key = %s", path, uploadPath)
	return nil
}

func (b Backup) pruneS3(backupType string) error {
	objects := b.getBucketObjects(backupType)
	var keys []string
	for _, o := range objects {
		keys = append(keys, aws.StringValue(o.Key))
	}

	sort.Sort(sort.Reverse(sort.StringSlice(keys)))

	numberToKeep := b.HourlyBackups
	switch backupType {
	case "daily":
		numberToKeep = b.DailyBackups
	case "weekly":
		numberToKeep = b.WeeklyBackups
	case "monthly":
		numberToKeep = b.MonthlyBackups
	}

	if len(keys) <= numberToKeep {
		log.Debug("Nothing to prune, skipping.")
		return nil
	}

	var deleteObjects []s3manager.BatchDeleteObject
	for _, k := range keys[numberToKeep:] {
		deleteObjects = append(deleteObjects, s3manager.BatchDeleteObject{
			Object: &s3.DeleteObjectInput{
				Bucket: aws.String(b.S3Bucket),
				Key:    aws.String(k),
			},
		})
	}

	batcher := s3manager.NewBatchDeleteWithClient(b.S3Service)
	if err := batcher.Delete(aws.BackgroundContext(), &s3manager.DeleteObjectsIterator{
		Objects: deleteObjects,
	}); err != nil {
		return err
	}

	log.Info("S3 backups pruned successfully")
	return nil
}

func (b Backup) removeBackupDirectory() error {
	dir := os.TempDir() + "/backups"
	return os.RemoveAll(dir)
}

func (b Backup) getBucketObjects(backupType string) []*s3.Object {
	i := &s3.ListObjectsInput{
		Bucket: aws.String(b.S3Bucket),
		Prefix: aws.String(fmt.Sprintf("%s/%s", b.S3Path, backupType)),
	}

	o, err := b.S3Service.ListObjects(i)
	if err != nil {
		log.Fatal(err)
	}

	return o.Contents
}

func (b Backup) addFile(tw *tar.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		if err := file.Close(); err != nil {
			log.Warnf("failed to close file as part of tar creation: error = %s", err)
		}
	}(file)

	if stat, err := file.Stat(); err == nil {
		header, err := tar.FileInfoHeader(stat, path)
		if err != nil {
			return err
		}

		header.Name = strings.ReplaceAll(path, b.DataDirectory+"/", "")

		log.Infof("Adding file: %s => %s", path, header.Name)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if _, err := io.Copy(tw, file); err != nil {
			return err
		}
	}

	return nil
}
