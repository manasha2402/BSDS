package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// ErrPhotoDeleted is returned by UpdatePhotoStatus when the photo record no
// longer exists — it was deleted while the worker was still uploading to S3.
var ErrPhotoDeleted = errors.New("photo deleted before worker completed")

type dynamoStore struct {
	client      *dynamodb.Client
	albumsTable string
	photosTable string
}

func initDynamo() *dynamoStore {
	region := getEnvOrDefault("AWS_REGION", "us-west-2")
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		log.Fatalf("dynamodb config: %v", err)
	}
	return &dynamoStore{
		client:      dynamodb.NewFromConfig(cfg),
		albumsTable: os.Getenv("ALBUMS_TABLE"),
		photosTable: os.Getenv("PHOTOS_TABLE"),
	}
}

// ─── Albums ────────────────────────────────────────────────────────────────────

// PutAlbum upserts an album. Returns true if the item was newly created (201).
//
// UpdateItem with if_not_exists(photo_seq, :zero) initialises the counter to 0
// on first create and leaves it unchanged on subsequent updates. ReturnValues
// ALL_OLD: empty map → new item; populated → existing item.
func (d *dynamoStore) PutAlbum(ctx context.Context, a Album) (isNew bool, err error) {
	out, err := d.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(d.albumsTable),
		Key: map[string]types.AttributeValue{
			"album_id": &types.AttributeValueMemberS{Value: a.AlbumID},
		},
		UpdateExpression: aws.String(
			"SET #t = :t, #d = :d, #o = :o, photo_seq = if_not_exists(photo_seq, :zero)",
		),
		ExpressionAttributeNames: map[string]string{
			"#t": "title",
			"#d": "description",
			"#o": "owner",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":t":    &types.AttributeValueMemberS{Value: a.Title},
			":d":    &types.AttributeValueMemberS{Value: a.Description},
			":o":    &types.AttributeValueMemberS{Value: a.Owner},
			":zero": &types.AttributeValueMemberN{Value: "0"},
		},
		ReturnValues: types.ReturnValueAllOld,
	})
	if err != nil {
		return false, err
	}
	return len(out.Attributes) == 0, nil
}

// GetAlbum returns the album or nil if not found.
func (d *dynamoStore) GetAlbum(ctx context.Context, albumID string) (*Album, error) {
	out, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.albumsTable),
		Key: map[string]types.AttributeValue{
			"album_id": &types.AttributeValueMemberS{Value: albumID},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return nil, nil
	}
	return itemToAlbum(out.Item), nil
}

// ListAlbums returns all albums. Paginates through DynamoDB's 1 MB scan limit.
func (d *dynamoStore) ListAlbums(ctx context.Context) ([]Album, error) {
	albums := make([]Album, 0)
	var lastKey map[string]types.AttributeValue
	for {
		out, err := d.client.Scan(ctx, &dynamodb.ScanInput{
			TableName:         aws.String(d.albumsTable),
			ExclusiveStartKey: lastKey,
		})
		if err != nil {
			return nil, err
		}
		for _, item := range out.Items {
			if a := itemToAlbum(item); a != nil {
				albums = append(albums, *a)
			}
		}
		if out.LastEvaluatedKey == nil {
			break
		}
		lastKey = out.LastEvaluatedKey
	}
	return albums, nil
}

// ─── Photos ────────────────────────────────────────────────────────────────────

// IncrementPhotoSeq atomically increments the album's photo_seq counter and
// returns the new value. Works across all ECS tasks without transactions or
// row locks — DynamoDB serialises ADD operations at the item level.
func (d *dynamoStore) IncrementPhotoSeq(ctx context.Context, albumID string) (int, error) {
	out, err := d.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(d.albumsTable),
		Key: map[string]types.AttributeValue{
			"album_id": &types.AttributeValueMemberS{Value: albumID},
		},
		UpdateExpression:    aws.String("ADD photo_seq :inc"),
		ConditionExpression: aws.String("attribute_exists(album_id)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":inc": &types.AttributeValueMemberN{Value: "1"},
		},
		ReturnValues: types.ReturnValueUpdatedNew,
	})
	if err != nil {
		return 0, err
	}
	seqAttr, ok := out.Attributes["photo_seq"].(*types.AttributeValueMemberN)
	if !ok {
		return 0, fmt.Errorf("unexpected photo_seq type in response")
	}
	return strconv.Atoi(seqAttr.Value)
}

// PutPhoto inserts a new photo record as 'processing'.
func (d *dynamoStore) PutPhoto(ctx context.Context, p Photo) error {
	_, err := d.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(d.photosTable),
		Item: map[string]types.AttributeValue{
			"photo_id": &types.AttributeValueMemberS{Value: p.PhotoID},
			"album_id": &types.AttributeValueMemberS{Value: p.AlbumID},
			"seq":      &types.AttributeValueMemberN{Value: strconv.Itoa(p.Seq)},
			"status":   &types.AttributeValueMemberS{Value: p.Status},
			"s3_key":   &types.AttributeValueMemberS{Value: p.S3Key},
		},
	})
	return err
}

// GetPhoto returns the photo or nil if not found.
func (d *dynamoStore) GetPhoto(ctx context.Context, photoID string) (*Photo, error) {
	out, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.photosTable),
		Key: map[string]types.AttributeValue{
			"photo_id": &types.AttributeValueMemberS{Value: photoID},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return nil, nil
	}
	return itemToPhoto(out.Item), nil
}

// DeletePhoto atomically deletes the photo and returns its record.
// Returns nil if the photo didn't exist or belonged to a different album (idempotent).
func (d *dynamoStore) DeletePhoto(ctx context.Context, photoID, albumID string) (*Photo, error) {
	out, err := d.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(d.photosTable),
		Key: map[string]types.AttributeValue{
			"photo_id": &types.AttributeValueMemberS{Value: photoID},
		},
		ConditionExpression: aws.String("album_id = :aid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":aid": &types.AttributeValueMemberS{Value: albumID},
		},
		ReturnValues: types.ReturnValueAllOld,
	})
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return nil, nil // already deleted or wrong album — treat as not found
		}
		return nil, err
	}
	return itemToPhoto(out.Attributes), nil
}

// UpdatePhotoStatus sets the photo's status. Returns ErrPhotoDeleted if the
// photo record no longer exists (deleted while worker was uploading).
// The ConditionExpression prevents re-creating a deleted photo record.
func (d *dynamoStore) UpdatePhotoStatus(ctx context.Context, photoID, status string) error {
	_, err := d.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(d.photosTable),
		Key: map[string]types.AttributeValue{
			"photo_id": &types.AttributeValueMemberS{Value: photoID},
		},
		UpdateExpression:    aws.String("SET #s = :s"),
		ConditionExpression: aws.String("attribute_exists(photo_id)"),
		ExpressionAttributeNames: map[string]string{
			"#s": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":s": &types.AttributeValueMemberS{Value: status},
		},
	})
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return ErrPhotoDeleted
		}
		return err
	}
	return nil
}

// ─── Item helpers ──────────────────────────────────────────────────────────────

// itemToAlbum converts a DynamoDB item map to an Album.
// Uses comma-ok assertions — returns nil for any partial/corrupt record.
func itemToAlbum(item map[string]types.AttributeValue) *Album {
	albumID, ok1 := item["album_id"].(*types.AttributeValueMemberS)
	title, ok2 := item["title"].(*types.AttributeValueMemberS)
	desc, ok3 := item["description"].(*types.AttributeValueMemberS)
	owner, ok4 := item["owner"].(*types.AttributeValueMemberS)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return nil
	}
	return &Album{
		AlbumID:     albumID.Value,
		Title:       title.Value,
		Description: desc.Value,
		Owner:       owner.Value,
	}
}

// itemToPhoto converts a DynamoDB item map to a Photo.
// Uses comma-ok assertions — returns nil for any partial/corrupt record
// (e.g. a record left half-updated by a crashed worker).
func itemToPhoto(item map[string]types.AttributeValue) *Photo {
	photoID, ok1 := item["photo_id"].(*types.AttributeValueMemberS)
	albumID, ok2 := item["album_id"].(*types.AttributeValueMemberS)
	seqAttr, ok3 := item["seq"].(*types.AttributeValueMemberN)
	statusAttr, ok4 := item["status"].(*types.AttributeValueMemberS)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return nil // partial record — caller treats as not found
	}
	seq, _ := strconv.Atoi(seqAttr.Value)
	p := &Photo{
		PhotoID: photoID.Value,
		AlbumID: albumID.Value,
		Seq:     seq,
		Status:  statusAttr.Value,
	}
	if s3k, ok := item["s3_key"].(*types.AttributeValueMemberS); ok {
		p.S3Key = s3k.Value
	}
	return p
}
