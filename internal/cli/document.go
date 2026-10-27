package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	doccap "github.com/eosaios/eos/internal/document"
	"github.com/spf13/cobra"
)

func newDocumentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doc",
		Short: "Read, generate and convert DOCX/XLSX/PDF documents.",
	}

	var readJSON bool
	readCmd := &cobra.Command{
		Use:   "read <path>",
		Short: "Read a DOCX/XLSX/PDF file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			format := doccap.NormalizeFormat(path)
			var payload any
			var text string
			switch format {
			case "docx":
				model, err := doccap.ReadDOCX(path)
				if err != nil {
					return err
				}
				payload = model
				text = model.PlainText()
			case "xlsx":
				model, err := doccap.ReadXLSX(path)
				if err != nil {
					return err
				}
				payload = model
				text = model.PlainText()
			case "pdf":
				model, err := doccap.ReadPDF(path)
				if err != nil {
					return err
				}
				payload = model
				text = model.PlainText()
			default:
				return fmt.Errorf("unsupported format: %s", path)
			}
			if readJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}
			_, err := fmt.Fprintln(os.Stdout, text)
			return err
		},
	}
	readCmd.Flags().BoolVar(&readJSON, "json", false, "print structured JSON")

	var genFormat, genOutput, genTitle, genContent string
	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a DOCX/XLSX/PDF file from plain text",
		RunE: func(cmd *cobra.Command, args []string) error {
			format := doccap.NormalizeFormat(genFormat)
			if format == "" {
				return fmt.Errorf("--format must be docx/xlsx/pdf")
			}
			if strings.TrimSpace(genOutput) == "" {
				return fmt.Errorf("--output is required")
			}
			switch format {
			case "docx":
				return doccap.WriteDOCX(genOutput, doccap.DocumentFromText(genTitle, genContent))
			case "pdf":
				return doccap.WritePDF(genOutput, doccap.DocumentFromText(genTitle, genContent))
			case "xlsx":
				return doccap.WriteXLSX(genOutput, doccap.WorkbookFromText(genTitle, genContent))
			default:
				return fmt.Errorf("unsupported format: %s", format)
			}
		},
	}
	generateCmd.Flags().StringVar(&genFormat, "format", "", "target format: docx/xlsx/pdf")
	generateCmd.Flags().StringVar(&genOutput, "output", "", "output file path")
	generateCmd.Flags().StringVar(&genTitle, "title", "", "document title")
	generateCmd.Flags().StringVar(&genContent, "content", "", "document body content")

	var convOutput, convTarget, convFidelity string
	convertCmd := &cobra.Command{
		Use:   "convert <source>",
		Short: "Convert a DOCX/XLSX/PDF file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := doccap.Convert(args[0], doccap.ConversionOptions{DestinationPath: convOutput, TargetFormat: convTarget, Fidelity: convFidelity})
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		},
	}
	convertCmd.Flags().StringVar(&convOutput, "output", "", "output file path")
	convertCmd.Flags().StringVar(&convTarget, "to", "", "target format: docx/xlsx/pdf")
	convertCmd.Flags().StringVar(&convFidelity, "fidelity", "high", "conversion fidelity: high/content")

	cmd.AddCommand(readCmd, generateCmd, convertCmd)
	return cmd
}
