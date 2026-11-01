package update

// Copyright (c) 2026 EOSAIOS
// SPDX-License-Identifier: EOS-NCL-1.1
// 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
// 商业使用请联系版权人获得商业授权。

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// 所有组件先复制到安装文件系统，再通过 rename 替换；下载临时目录可能
// 位于不同卷。旧组件保留到全部成功，Windows 运行中的程序也可改名备份。
func installRelease(root, newBinary, executable string, install Installation) error {
	dir := filepath.Dir(executable)
	stage, err := os.MkdirTemp(dir, ".eos-update-")
	if err != nil {
		return fmt.Errorf("stage release in installation directory: %w", err)
	}
	transaction := releaseTransaction{stage: stage, dir: dir, rename: os.Rename}
	defer func() {
		// Windows 仍在运行的旧程序可能暂时不可删除；保留备份不影响新包。
		if !transaction.recoveryRequired {
			_ = os.RemoveAll(stage)
		}
	}()
	binaryName := filepath.Base(executable)
	if err := copyExecutable(newBinary, filepath.Join(stage, binaryName)); err != nil {
		return fmt.Errorf("stage release binary: %w", err)
	}
	if err := copyDir(filepath.Join(root, "core"), filepath.Join(stage, "core")); err != nil {
		return fmt.Errorf("stage release core: %w", err)
	}
	pair, err := os.ReadFile(filepath.Join(root, releasePairFile))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, releasePairFile), pair, 0o644); err != nil {
		return fmt.Errorf("stage release pair: %w", err)
	}
	if err := WriteInstallation(filepath.Join(stage, binaryName), install); err != nil {
		return fmt.Errorf("stage installation channel: %w", err)
	}
	return transaction.commit([]string{binaryName, "core", releasePairFile, InstallationFile})
}

type releaseTransaction struct {
	stage            string
	dir              string
	rename           func(string, string) error
	recoveryRequired bool
}

type replacedComponent struct {
	name      string
	backup    string
	installed bool
}

func (tx *releaseTransaction) commit(names []string) error {
	var replaced []replacedComponent
	for _, name := range names {
		target := filepath.Join(tx.dir, name)
		component := replacedComponent{name: name}
		if _, err := os.Lstat(target); err == nil {
			component.backup = filepath.Join(tx.stage, "previous-"+name)
			if err := tx.rename(target, component.backup); err != nil {
				return tx.rollback(replaced, fmt.Errorf("backup %s: %w", name, err))
			}
		} else if !os.IsNotExist(err) {
			return tx.rollback(replaced, fmt.Errorf("inspect %s: %w", name, err))
		}
		replaced = append(replaced, component)
		if err := tx.rename(filepath.Join(tx.stage, name), target); err != nil {
			return tx.rollback(replaced, fmt.Errorf("install %s: %w", name, err))
		}
		replaced[len(replaced)-1].installed = true
	}
	return nil
}

func (tx *releaseTransaction) rollback(replaced []replacedComponent, cause error) error {
	for i := len(replaced) - 1; i >= 0; i-- {
		component := replaced[i]
		target := filepath.Join(tx.dir, component.name)
		// 将新组件移回 staging，避免 Windows 下删除正在运行文件的限制。
		if component.installed {
			if err := tx.rename(target, filepath.Join(tx.stage, component.name)); err != nil {
				tx.recoveryRequired = true
				cause = errors.Join(cause, fmt.Errorf("rollback %s: %w", component.name, err))
				continue
			}
		}
		if component.backup != "" {
			if err := tx.rename(component.backup, target); err != nil {
				tx.recoveryRequired = true
				cause = errors.Join(cause, fmt.Errorf("restore %s: %w", component.name, err))
			}
		}
	}
	if tx.recoveryRequired {
		return errors.Join(cause, fmt.Errorf("recovery files preserved at %s", tx.stage))
	}
	return cause
}
