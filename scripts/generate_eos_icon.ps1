# 已废弃（2026-10-08）：本脚本是 Windows 时代「去白底做剪影」的旧图标管线，
# 产物为透明底剪影而非家族白卡全出血标准，禁止再用于生成图标。
# 现行标准见 eos-docs(品牌仓) brand-guidelines.md 第七节；eos-app 构建期有全出血校验门。
param(
  [string]$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path,
  [string]$Source = 'C:\home\eos\eos.png'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

Add-Type -AssemblyName System.Drawing

$processorSource = @'
using System;
using System.Collections.Generic;
using System.Drawing;
using System.Drawing.Drawing2D;
using System.Drawing.Imaging;
using System.Runtime.InteropServices;

public static class EosIconProcessor
{
    public static Bitmap CreateTransparentBitmap(string sourcePath, int size)
    {
        using (var source = Image.FromFile(sourcePath))
        {
            var bitmap = new Bitmap(size, size, PixelFormat.Format32bppArgb);
            using (var graphics = Graphics.FromImage(bitmap))
            {
                graphics.Clear(Color.Transparent);
                graphics.CompositingQuality = CompositingQuality.HighQuality;
                graphics.InterpolationMode = InterpolationMode.HighQualityBicubic;
                graphics.PixelOffsetMode = PixelOffsetMode.HighQuality;
                graphics.SmoothingMode = SmoothingMode.HighQuality;
                graphics.DrawImage(source, new Rectangle(0, 0, size, size));
            }
            RemoveConnectedBackground(bitmap);
            return bitmap;
        }
    }

    public static Bitmap Resize(Bitmap source, int size)
    {
        var bitmap = new Bitmap(size, size, PixelFormat.Format32bppArgb);
        using (var graphics = Graphics.FromImage(bitmap))
        {
            graphics.Clear(Color.Transparent);
            graphics.CompositingQuality = CompositingQuality.HighQuality;
            graphics.InterpolationMode = InterpolationMode.HighQualityBicubic;
            graphics.PixelOffsetMode = PixelOffsetMode.HighQuality;
            graphics.SmoothingMode = SmoothingMode.HighQuality;
            graphics.DrawImage(source, new Rectangle(0, 0, size, size));
        }
        return bitmap;
    }

    private static void RemoveConnectedBackground(Bitmap bitmap)
    {
        var rect = new Rectangle(0, 0, bitmap.Width, bitmap.Height);
        var data = bitmap.LockBits(rect, ImageLockMode.ReadWrite, PixelFormat.Format32bppArgb);
        try
        {
            int stride = data.Stride;
            int width = bitmap.Width;
            int height = bitmap.Height;
            int length = Math.Abs(stride) * height;
            byte[] pixels = new byte[length];
            Marshal.Copy(data.Scan0, pixels, 0, length);

            bool[] seen = new bool[width * height];
            var queue = new Queue<int>(width * 2 + height * 2);

            Action<int, int> enqueue = (x, y) =>
            {
                if (x < 0 || y < 0 || x >= width || y >= height) return;
                int index = y * width + x;
                if (seen[index]) return;
                int offset = y * stride + x * 4;
                if (offset < 0 || offset + 3 >= pixels.Length) return;
                byte b = pixels[offset];
                byte g = pixels[offset + 1];
                byte r = pixels[offset + 2];
                byte a = pixels[offset + 3];
                if (a == 0 || IsBackgroundCandidate(r, g, b))
                {
                    seen[index] = true;
                    queue.Enqueue(index);
                }
            };

            for (int x = 0; x < width; x++)
            {
                enqueue(x, 0);
                enqueue(x, height - 1);
            }
            for (int y = 1; y < height - 1; y++)
            {
                enqueue(0, y);
                enqueue(width - 1, y);
            }

            while (queue.Count > 0)
            {
                int index = queue.Dequeue();
                int x = index % width;
                int y = index / width;
                int offset = y * stride + x * 4;
                pixels[offset] = 0;
                pixels[offset + 1] = 0;
                pixels[offset + 2] = 0;
                pixels[offset + 3] = 0;

                enqueue(x + 1, y);
                enqueue(x - 1, y);
                enqueue(x, y + 1);
                enqueue(x, y - 1);
            }

            Marshal.Copy(pixels, 0, data.Scan0, length);
        }
        finally
        {
            bitmap.UnlockBits(data);
        }
    }

    private static bool IsBackgroundCandidate(byte r, byte g, byte b)
    {
        int max = Math.Max(r, Math.Max(g, b));
        int min = Math.Min(r, Math.Min(g, b));
        int luma = (r * 299 + g * 587 + b * 114) / 1000;
        return luma >= 210 && (max - min) <= 58 && r >= 196 && g >= 196 && b >= 190;
    }
}
'@

if (-not ('EosIconProcessor' -as [type])) {
  Add-Type -TypeDefinition $processorSource -ReferencedAssemblies System.Drawing
}

function Convert-BitmapToPngBytes {
  param([System.Drawing.Bitmap]$Bitmap)
  $stream = [System.IO.MemoryStream]::new()
  try {
    $Bitmap.Save($stream, [System.Drawing.Imaging.ImageFormat]::Png)
    return $stream.ToArray()
  } finally {
    $stream.Dispose()
  }
}

function Write-UInt16LE {
  param([System.IO.Stream]$Stream, [int]$Value)
  $Stream.WriteByte($Value -band 0xff)
  $Stream.WriteByte(($Value -shr 8) -band 0xff)
}

function Write-UInt32LE {
  param([System.IO.Stream]$Stream, [int64]$Value)
  $Stream.WriteByte($Value -band 0xff)
  $Stream.WriteByte(($Value -shr 8) -band 0xff)
  $Stream.WriteByte(($Value -shr 16) -band 0xff)
  $Stream.WriteByte(($Value -shr 24) -band 0xff)
}

function New-IcoBytes {
  param(
    [System.Drawing.Bitmap]$SourceBitmap,
    [int[]]$Sizes
  )

  $entries = @()
  foreach ($size in $Sizes) {
    $bitmap = [EosIconProcessor]::Resize($SourceBitmap, $size)
    try {
      $entries += [pscustomobject]@{
        Size = $size
        Bytes = Convert-BitmapToPngBytes $bitmap
      }
    } finally {
      $bitmap.Dispose()
    }
  }

  $iconStream = [System.IO.MemoryStream]::new()
  try {
    Write-UInt16LE $iconStream 0
    Write-UInt16LE $iconStream 1
    Write-UInt16LE $iconStream $entries.Count

    $offset = 6 + (16 * $entries.Count)
    foreach ($entry in $entries) {
      $iconStream.WriteByte($(if ($entry.Size -eq 256) { 0 } else { $entry.Size }))
      $iconStream.WriteByte($(if ($entry.Size -eq 256) { 0 } else { $entry.Size }))
      $iconStream.WriteByte(0)
      $iconStream.WriteByte(0)
      Write-UInt16LE $iconStream 1
      Write-UInt16LE $iconStream 32
      Write-UInt32LE $iconStream $entry.Bytes.Length
      Write-UInt32LE $iconStream $offset
      $offset += $entry.Bytes.Length
    }

    foreach ($entry in $entries) {
      $iconStream.Write($entry.Bytes, 0, $entry.Bytes.Length)
    }
    return $iconStream.ToArray()
  } finally {
    $iconStream.Dispose()
  }
}

if (-not (Test-Path -LiteralPath $Source)) {
  throw "Icon source not found: $Source"
}

$assetsDir = Join-Path $Root 'assets'
New-Item -ItemType Directory -Force -Path $assetsDir | Out-Null

$preview = [EosIconProcessor]::CreateTransparentBitmap($Source, 1024)
try {
  $pngOutputs = @((Join-Path $assetsDir 'eos-icon.png'))
  $frontendPublic = Join-Path $Root 'frontend\public'
  if (Test-Path -LiteralPath $frontendPublic) {
    New-Item -ItemType Directory -Force -Path $frontendPublic | Out-Null
    $pngOutputs += (Join-Path $frontendPublic 'eos-icon.png')
  }

  foreach ($pngPath in $pngOutputs) {
    $preview.Save($pngPath, [System.Drawing.Imaging.ImageFormat]::Png)
    Write-Host "Generated $pngPath"
  }

  $icoBytes = New-IcoBytes $preview @(16, 24, 32, 48, 64, 128, 256)
  $icoOutputs = @()
  if (Test-Path -LiteralPath (Join-Path $Root 'installer.iss')) {
    $icoOutputs += (Join-Path $Root 'eos.ico')
  } else {
    $icoOutputs += (Join-Path $assetsDir 'eos.ico')
  }

  foreach ($icoPath in $icoOutputs) {
    [System.IO.File]::WriteAllBytes($icoPath, $icoBytes)
    Write-Host "Generated $icoPath"
  }

  $legacySvg = Join-Path $assetsDir 'eos-icon.svg'
  if (Test-Path -LiteralPath $legacySvg) {
    Remove-Item -LiteralPath $legacySvg -Force
    Write-Host "Removed $legacySvg"
  }
} finally {
  $preview.Dispose()
}
