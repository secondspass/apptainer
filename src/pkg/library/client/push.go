/*
  Copyright (c) 2018, Sylabs, Inc. All rights reserved.

  This software is licensed under a 3-clause BSD license.  Please
  consult LICENSE file distributed with the sources of this project regarding
  your rights to use or distribute this software.
*/

package client

import (
	"bufio"
	"fmt"
	"net/http"
	"os"

	"github.com/singularityware/singularity/src/pkg/sylog"

	"gopkg.in/cheggaaa/pb.v1"
)

// UploadImage will push a specified image up to the Container Library,
func UploadImage(filePath string, libraryRef string, libraryURL string) error {

	if !isLibraryPushRef(libraryRef) {
		return fmt.Errorf("Not a valid library reference: %s", libraryRef)
	}

	imageHash, err := ImageHash(filePath)
	if err != nil {
		return err
	}
	sylog.Debugf("Image hash computed as %s\n", imageHash)

	entityName, collectionName, containerName, tags := parseLibraryRef(libraryRef)

	// Find or create entity
	entity, found, err := getEntity(libraryURL, entityName)
	if err != nil {
		return err
	}
	if !found {
		sylog.Verbosef("Entity %s does not exist in library - creating it.\n", entityName)
		entity, err = createEntity(libraryURL, entityName)
		if err != nil {
			return err
		}
	}

	// Find or create collection
	collection, found, err := getCollection(libraryURL, entityName+"/"+collectionName)
	if err != nil {
		return err
	}
	if !found {
		sylog.Verbosef("Collection %s does not exist in library - creating it.\n", collectionName)
		collection, err = createCollection(libraryURL, collectionName, entity.GetID().Hex())
		if err != nil {
			return err
		}
	}

	// Find or create container
	container, found, err := getContainer(libraryURL, entityName+"/"+collectionName+"/"+containerName)
	if err != nil {
		return err
	}
	if !found {
		sylog.Verbosef("Container %s does not exist in library - creating it.\n", containerName)
		container, err = createContainer(libraryURL, containerName, collection.GetID().Hex())
		if err != nil {
			return err
		}
	}

	// Find or create image
	image, found, err := getImage(libraryURL, entityName+"/"+collectionName+"/"+containerName+":"+imageHash)
	if err != nil {
		return err
	}
	if !found {
		sylog.Verbosef("Image %s does not exist in library - creating it.\n", imageHash)
		image, err = createImage(libraryURL, imageHash, container.GetID().Hex())
		if err != nil {
			return err
		}
	}

	if !image.Uploaded {
		sylog.Infof("Now uploading %s to the library\n", filePath)
		err = postFile(libraryURL, filePath, image.GetID().Hex())
		if err != nil {
			return err
		}
		sylog.Debugf("Upload completed OK\n")
	} else {
		sylog.Infof("Image is already present in the library - not uploading.\n")
	}

	sylog.Debugf("Setting tags against uploaded image\n")
	err = setTags(libraryURL, container.GetID().Hex(), image.GetID().Hex(), tags)
	if err != nil {
		return err
	}

	return nil
}

func postFile(baseURL string, filePath string, imageID string) error {

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("Could not open the image file to upload: %v", err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("Could not find size of the image file to upload: %v", err)
	}
	fileSize := fi.Size()

	postURL := baseURL + "/v1/imagefile/" + imageID
	sylog.Debugf("postFile calling %s\n", postURL)

	b := bufio.NewReader(f)

	// create and start bar
	bar := pb.New(int(fileSize)).SetUnits(pb.U_BYTES)
	bar.ShowTimeLeft = true
	bar.ShowSpeed = true
	bar.Start()
	// create proxy reader
	bodyProgress := bar.NewProxyReader(b)
	// Make an upload request
	req, _ := http.NewRequest("POST", postURL, bodyProgress)
	// Don't forget to set the content type, this will contain the boundary.
	req.Header.Set("Content-Type", "application/octet-stream")
	// Content length is required by the API
	req.ContentLength = fileSize
	client := &http.Client{}
	res, err := client.Do(req)

	bar.Finish()

	if err != nil {
		return fmt.Errorf("Error uploading file to server: %s", err.Error())
	}
	if res.StatusCode != http.StatusOK {
		jRes, err := ParseErrorBody(res.Body)
		if err != nil {
			jRes = ParseErrorResponse(res)
		}
		return fmt.Errorf("Sending file did not succeed: %d %s\n\t%v",
			jRes.Error.Code, jRes.Error.Status, jRes.Error.Message)
	}

	return nil

}
