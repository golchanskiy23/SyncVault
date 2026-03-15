package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/options"

	"syncvault/internal/domain/ports"
	"syncvault/internal/domain/valueobjects"
)

type GridFSAdapter struct {
	bucket    *gridfs.Bucket
	database  *mongo.Database
	chunkSize int32
}

func NewGridFSAdapter(db *mongo.Database, bucketName string) *GridFSAdapter {
	chunkSize := int32(255 * 1024)
	opts := options.GridFSBucket().
		SetName(bucketName).
		SetChunkSizeBytes(chunkSize)

	bucket, err := gridfs.NewBucket(db, opts)
	if err != nil {
		log.Fatalf("Failed to create GridFS bucket: %v", err)
	}

	return &GridFSAdapter{
		bucket:    bucket,
		database:  db,
		chunkSize: chunkSize,
	}
}

func (g *GridFSAdapter) PutFile(ctx context.Context, filePath valueobjects.FilePath, content io.Reader, size int64) error {
	uploadOpts := options.MergeUploadOptions(
		options.GridFSUpload().
			SetChunkSizeBytes(g.chunkSize).
			SetMetadata(bson.M{
				"uploadTime": time.Now().UTC(),
				"size":       size,
			}),
	)

	uploadStream, err := g.bucket.OpenUploadStream(filePath.String(), uploadOpts)
	if err != nil {
		return fmt.Errorf("failed to open upload stream: %w", err)
	}
	defer uploadStream.Close()

	written, err := io.Copy(uploadStream, content)
	if err != nil {
		return fmt.Errorf("failed to copy content to GridFS: %w", err)
	}

	log.Printf("✓ Stored file in GridFS: %s (%d bytes)", filePath.String(), written)
	return nil
}

func (g *GridFSAdapter) GetFile(ctx context.Context, filePath valueobjects.FilePath) (io.ReadCloser, int64, error) {
	type gridfsFile struct {
		ID     interface{} `bson:"_id"`
		Length int64       `bson:"length"`
	}

	cursor, err := g.bucket.Find(bson.M{"filename": filePath.String()})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to find file in GridFS: %w", err)
	}
	defer cursor.Close(ctx)

	var fileDoc gridfsFile
	if !cursor.Next(ctx) {
		return nil, 0, fmt.Errorf("file not found in GridFS: %s", filePath.String())
	}
	if err := cursor.Decode(&fileDoc); err != nil {
		return nil, 0, fmt.Errorf("failed to decode file document: %w", err)
	}

	downloadStream, err := g.bucket.OpenDownloadStream(fileDoc.ID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open download stream: %w", err)
	}

	return downloadStream, fileDoc.Length, nil
}

func (g *GridFSAdapter) DeleteFile(ctx context.Context, filePath valueobjects.FilePath) error {
	type gridfsFile struct {
		ID interface{} `bson:"_id"`
	}

	cursor, err := g.bucket.Find(bson.M{"filename": filePath.String()})
	if err != nil {
		return fmt.Errorf("failed to find file in GridFS: %w", err)
	}
	defer cursor.Close(ctx)

	var fileDoc gridfsFile
	if !cursor.Next(ctx) {
		return fmt.Errorf("file not found in GridFS: %s", filePath.String())
	}
	if err := cursor.Decode(&fileDoc); err != nil {
		return fmt.Errorf("failed to decode file document: %w", err)
	}

	if err := g.bucket.Delete(fileDoc.ID); err != nil {
		return fmt.Errorf("failed to delete file from GridFS: %w", err)
	}

	log.Printf("✓ Deleted file from GridFS: %s", filePath.String())
	return nil
}

func (g *GridFSAdapter) FileExists(ctx context.Context, filePath valueobjects.FilePath) (bool, error) {
	cursor, err := g.bucket.Find(bson.M{"filename": filePath.String()})
	if err != nil {
		return false, fmt.Errorf("failed to find file in GridFS: %w", err)
	}
	defer cursor.Close(ctx)

	found := cursor.Next(ctx)
	return found, cursor.Err()
}

func (g *GridFSAdapter) CreateDir(ctx context.Context, path valueobjects.FilePath) error {
	uploadOpts := options.GridFSUpload().
		SetChunkSizeBytes(g.chunkSize).
		SetMetadata(bson.M{
			"type":      "directory",
			"createdAt": time.Now().UTC(),
		})

	uploadStream, err := g.bucket.OpenUploadStream(path.String()+"/.dir", uploadOpts)
	if err != nil {
		return fmt.Errorf("failed to create directory placeholder: %w", err)
	}
	defer uploadStream.Close()

	return nil
}

func (g *GridFSAdapter) ListDir(ctx context.Context, path valueobjects.FilePath) ([]ports.FileInfo, error) {
	cursor, err := g.bucket.Find(bson.M{"filename": bson.M{"$regex": "^" + path.String()}})
	if err != nil {
		return nil, fmt.Errorf("failed to list files in GridFS: %w", err)
	}
	defer cursor.Close(ctx)

	type gridfsFile struct {
		Name       string    `bson:"filename"`
		Length     int64     `bson:"length"`
		UploadDate time.Time `bson:"uploadDate"`
	}

	var files []ports.FileInfo
	for cursor.Next(ctx) {
		var fileDoc gridfsFile
		if err := cursor.Decode(&fileDoc); err != nil {
			continue
		}
		fp, err := valueobjects.NewFilePath(fileDoc.Name)
		if err != nil {
			continue
		}
		files = append(files, ports.FileInfo{
			Path:       fp,
			Size:       fileDoc.Length,
			ModifiedAt: fileDoc.UploadDate,
		})
	}

	return files, cursor.Err()
}

func (g *GridFSAdapter) GetFileInfo(ctx context.Context, filePath valueobjects.FilePath) (*ports.FileInfo, error) {
	type gridfsFile struct {
		Length     int64     `bson:"length"`
		UploadDate time.Time `bson:"uploadDate"`
	}

	cursor, err := g.bucket.Find(bson.M{"filename": filePath.String()})
	if err != nil {
		return nil, fmt.Errorf("failed to find file in GridFS: %w", err)
	}
	defer cursor.Close(ctx)

	var fileDoc gridfsFile
	if !cursor.Next(ctx) {
		return nil, fmt.Errorf("file not found in GridFS: %s", filePath.String())
	}
	if err := cursor.Decode(&fileDoc); err != nil {
		return nil, fmt.Errorf("failed to decode file document: %w", err)
	}

	return &ports.FileInfo{
		Path:       filePath,
		Size:       fileDoc.Length,
		ModifiedAt: fileDoc.UploadDate,
	}, nil
}

func (g *GridFSAdapter) GetSpaceInfo(ctx context.Context) (*ports.SpaceInfo, error) {
	type gridfsFile struct {
		Length int64 `bson:"length"`
	}

	cursor, err := g.bucket.Find(bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to query GridFS: %w", err)
	}
	defer cursor.Close(ctx)

	var totalSize int64
	for cursor.Next(ctx) {
		var f gridfsFile
		if err := cursor.Decode(&f); err == nil {
			totalSize += f.Length
		}
	}

	return &ports.SpaceInfo{
		UsedSpace: totalSize,
	}, nil
}

func (g *GridFSAdapter) Connect(ctx context.Context) error {
	log.Println("GridFS adapter connected")
	return nil
}

func (g *GridFSAdapter) Disconnect(ctx context.Context) error {
	log.Println("GridFS adapter disconnected")
	return nil
}

func (g *GridFSAdapter) IsConnected(ctx context.Context) bool {
	return g.bucket != nil
}

var _ ports.Storage = (*GridFSAdapter)(nil)
