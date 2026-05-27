/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"

	gothic_cli "github.com/felipegenef/gothicframework/v2/pkg/cli"
	"github.com/spf13/cobra"

	"github.com/nfnt/resize"
	webp "golang.org/x/image/webp"
)

var optimizeImagesCmd = &cobra.Command{
	Use:   "optimize-images",
	Short: "Generate optimized and blurred image variants in the public folder",
	Long: `Scans the "optimize" folder and generates optimized image assets in the "public" directory. 
For each image, a corresponding folder is created containing:

  - The original image
  - A blurred version (default: 20% of the original resolution)

You can customize the blur resolution by modifying the relevant setting in the "gothic-config.json" file.`,
	RunE: newOptimizeImagesCommand(gothic_cli.NewCli()),
}

func init() {
	rootCmd.AddCommand(optimizeImagesCmd)
}

type ImgOptimizationCommand struct {
	cli       *gothic_cli.GothicCli
	inputDir  string
	outputDir string
}

func NewImgOptimizationCommandCli(cli *gothic_cli.GothicCli) ImgOptimizationCommand {
	return ImgOptimizationCommand{
		cli:       cli,
		inputDir:  "./optimize",
		outputDir: "./public",
	}
}

func newOptimizeImagesCommand(cli gothic_cli.GothicCli) RunEFunc {
	return func(cmd *cobra.Command, args []string) error {
		command := NewImgOptimizationCommandCli(&cli)

		return command.OptimizeImages()
	}
}

func (command *ImgOptimizationCommand) OptimizeImages() error {

	config, err := command.cli.GetConfig()
	if err != nil {
		return err
	}

	// Create the output directory if it doesn't exist
	if err := os.MkdirAll(command.outputDir, os.ModePerm); err != nil {
		return err
	}

	// Read the files from the input directory
	files, err := os.ReadDir(command.inputDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if !file.IsDir() {
			inputPath := filepath.Join(command.inputDir, file.Name())
			baseName := file.Name()[:len(file.Name())-len(filepath.Ext(file.Name()))]
			imageDir := filepath.Join(command.outputDir, baseName)
			ext := filepath.Ext(file.Name())

			if err := os.MkdirAll(imageDir, os.ModePerm); err != nil {
				return fmt.Errorf("error creating directory %s: %v", imageDir, err)
			}

			// Open the image file
			imgFile, err := os.Open(inputPath)
			if err != nil {
				return fmt.Errorf("error opening file %s: %v", inputPath, err)
			}

			// Decode the image
			var img image.Image
			switch ext {
			case ".png":
				img, err = png.Decode(imgFile)
			case ".jpg", ".jpeg":
				img, err = jpeg.Decode(imgFile)
			case ".webp":
				img, err = webp.Decode(imgFile)
			default:
				imgFile.Close()
				return fmt.Errorf("error unsupported file format: %s", ext)
			}
			imgFile.Close()
			if err != nil {
				return fmt.Errorf("error decoding image %s: %v", inputPath, err)
			}

			// Get original dimensions
			originalWidth := uint(img.Bounds().Dx())
			originalHeight := uint(img.Bounds().Dy())

			// Calculate new dimensions for blurred image (20% of original)
			// Check if Deploy section exists and get lowResolutionRate
			lowResolutionRate := 20 // default value
			if config.OptimizeImages.LowResolutionRate > 0 {
				lowResolutionRate = config.OptimizeImages.LowResolutionRate
			}

			newWidth := originalWidth * uint(lowResolutionRate) / 100
			newHeight := originalHeight * uint(lowResolutionRate) / 100

			// Save the original image
			originalPath := filepath.Join(imageDir, "original"+ext)
			originalFile, err := os.Create(originalPath)
			if err != nil {
				return fmt.Errorf("error creating original file %s: %v", originalPath, err)
			}
			defer originalFile.Close()

			// Resize the image to 20% of its original dimensions
			resizedImg := resize.Resize(newWidth, newHeight, img, resize.Lanczos3)

			// Save the resized image as blurred image
			blurredPath := filepath.Join(imageDir, "blurred"+ext)
			blurredFile, err := os.Create(blurredPath)
			if err != nil {
				return fmt.Errorf("error creating blurred file %s: %v", blurredPath, err)
			}
			defer blurredFile.Close()

			switch ext {
			case ".png":
				if err := png.Encode(originalFile, img); err != nil {
					return fmt.Errorf("error saving original PNG image %s: %v", originalPath, err)
				}
				if err := png.Encode(blurredFile, resizedImg); err != nil {
					return fmt.Errorf("error saving blurred PNG image %s: %v", originalPath, err)
				}

			case ".jpg", ".jpeg":
				if err := jpeg.Encode(originalFile, img, &jpeg.Options{Quality: 100}); err != nil {
					return fmt.Errorf("error saving original JPEG image %s: %v", originalPath, err)
				}
				if err := jpeg.Encode(blurredFile, resizedImg, &jpeg.Options{Quality: 20}); err != nil {
					return fmt.Errorf("error saving blurred image %s: %v", blurredPath, err)
				}
			case ".webp":
				if err := png.Encode(originalFile, img); err != nil {
					return fmt.Errorf("error saving original WebP image %s: %v", originalPath, err)
				}
				if err := png.Encode(blurredFile, resizedImg); err != nil {
					return fmt.Errorf("error saving blurred WebP image %s: %v", originalPath, err)
				}
			}
		} else {
			return fmt.Errorf("error the 'optimizeImages' key was not found in gothic-config.json")
		}

	}

	fmt.Println("Resizing complete!")
	return nil
}
