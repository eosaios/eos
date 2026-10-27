package document

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/eosaios/eos/internal/pkg/utils"
)

func Convert(sourcePath string, opts ConversionOptions) (ConversionResult, error) {
	srcFormat := NormalizeFormat(sourcePath)
	targetFormat := NormalizeFormat(opts.TargetFormat)
	if srcFormat == "" {
		return ConversionResult{}, fmt.Errorf("unsupported source format: %s", sourcePath)
	}
	if targetFormat == "" {
		return ConversionResult{}, fmt.Errorf("unsupported target format: %s", opts.TargetFormat)
	}
	destination := strings.TrimSpace(opts.DestinationPath)
	if destination == "" {
		base := strings.TrimSuffix(sourcePath, filepath.Ext(sourcePath))
		destination = base + "." + targetFormat
	}
	fidelity := strings.ToLower(strings.TrimSpace(opts.Fidelity))
	if fidelity == "" {
		fidelity = "high"
	}
	if fidelity != "high" && fidelity != "content" {
		return ConversionResult{}, fmt.Errorf("unsupported fidelity: %s (allowed: high, content)", opts.Fidelity)
	}

	result := ConversionResult{
		SourcePath:      sourcePath,
		DestinationPath: destination,
		SourceFormat:    srcFormat,
		TargetFormat:    targetFormat,
	}

	if srcFormat == targetFormat {
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return result, err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return result, err
		}
		if err := os.WriteFile(destination, data, 0o644); err != nil {
			return result, err
		}
		result.UsedEngine = "copy"
		return result, nil
	}

	// 高保真转换走 soffice（LibreOffice）。用户显式要求 high 时，soffice 不可用或失败
	// 属于硬性失败——不再静默降级到 content-level（那是掩盖问题：用户要 high，给 content
	// 还返回 nil error，调用方无从感知）。改成显式报错，让上层决定是否改用 --fidelity content 重试。
	if fidelity == "high" {
		warn, err := convertWithSoffice(sourcePath, destination, targetFormat)
		if err != nil {
			return result, fmt.Errorf("high-fidelity conversion failed: %w (soffice 不可用或转换失败；如可接受降级，请用 --fidelity content 重试)", err)
		}
		result.UsedEngine = "soffice"
		if warn != "" {
			result.Warnings = append(result.Warnings, warn)
		}
		return result, nil
	}

	// fidelity == "content"：用户显式选择内容级转换，这是正当路径，不是兜底。
	if err := contentConvert(sourcePath, destination, srcFormat, targetFormat); err != nil {
		return result, err
	}
	result.UsedEngine = "content"
	return result, nil
}

// convertWithSoffice 调用 LibreOffice headless 做高保真转换。
// 返回 soffice 的 stdout/stderr（作为非致命 warning 文本）与可能的错误。
// soffice 缺失或执行失败都返回非 nil err，由调用方决定是否改走 content-level。
func convertWithSoffice(sourcePath, destinationPath, targetFormat string) (string, error) {
	sofficePath, err := exec.LookPath("soffice")
	if err != nil {
		return "", fmt.Errorf("soffice not found in PATH: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return "", err
	}
	outDir := filepath.Dir(destinationPath)
	cmd := utils.Command(sofficePath, "--headless", "--convert-to", targetFormat, "--outdir", outDir, sourcePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("soffice convert failed: %s", strings.TrimSpace(string(output)))
	}
	produced := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))+"."+targetFormat)
	if produced != destinationPath {
		data, err := os.ReadFile(produced)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(destinationPath, data, 0o644); err != nil {
			return "", err
		}
		// 临时产物清理：soffice 输出到 outDir 后我们已搬到目标路径，删掉原产物。
		// 这不是业务错误（即便删除失败也不影响已生成的目标文件），故忽略返回值。
		_ = os.Remove(produced)
	}
	return strings.TrimSpace(string(output)), nil
}

// contentConvert 做内容级转换（基于文档模型读写，丢失布局保真度）。
// 这是 --fidelity content 的显式执行路径，不是 high 失败后的兜底。
func contentConvert(sourcePath, destinationPath, srcFormat, targetFormat string) error {
	switch srcFormat {
	case "docx":
		doc, err := ReadDOCX(sourcePath)
		if err != nil {
			return err
		}
		switch targetFormat {
		case "pdf":
			return WritePDF(destinationPath, doc)
		case "xlsx":
			return WriteXLSX(destinationPath, ToWorkbookModelFromDocument(doc))
		}
	case "pdf":
		doc, err := ReadPDF(sourcePath)
		if err != nil {
			return err
		}
		switch targetFormat {
		case "docx":
			return WriteDOCX(destinationPath, doc)
		case "xlsx":
			return WriteXLSX(destinationPath, ToWorkbookModelFromDocument(doc))
		}
	case "xlsx":
		book, err := ReadXLSX(sourcePath)
		if err != nil {
			return err
		}
		switch targetFormat {
		case "pdf":
			return WritePDF(destinationPath, ToDocumentModelFromWorkbook(book))
		case "docx":
			return WriteDOCX(destinationPath, ToDocumentModelFromWorkbook(book))
		}
	}
	return fmt.Errorf("unsupported conversion: %s -> %s", srcFormat, targetFormat)
}
