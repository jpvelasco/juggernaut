# tests/v2/ProfileWriter.Tests.ps1 - Pester 5.x tests for lib/profile_writer.ps1.
# Focus: CRLF line-ending robustness for detection and removal.

BeforeAll {
    $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    . (Join-Path $repoRoot 'lib/profile_writer.ps1')
    $script:Fixtures = Join-Path $repoRoot 'tests/v2/fixtures'
}

Describe 'Test-ProfileWriterHasBlock' {
    It 'detects the block in the LF fixture' {
        $tmp = [IO.Path]::GetTempFileName()
        try {
            Copy-Item (Join-Path $script:Fixtures 'profile_crlf_unix.sh') $tmp -Force
            Test-ProfileWriterHasBlock -ProfileFile $tmp | Should -BeTrue
        } finally {
            Remove-Item $tmp -Force -ErrorAction SilentlyContinue
        }
    }
    It 'detects the block in the CRLF fixture' {
        $tmp = [IO.Path]::GetTempFileName()
        try {
            Copy-Item (Join-Path $script:Fixtures 'profile_crlf.sh') $tmp -Force
            Test-ProfileWriterHasBlock -ProfileFile $tmp | Should -BeTrue
        } finally {
            Remove-Item $tmp -Force -ErrorAction SilentlyContinue
        }
    }
    It 'returns false for a missing file' {
        Test-ProfileWriterHasBlock -ProfileFile 'C:\does\not\exist.sh' | Should -BeFalse
    }
}

Describe 'Remove-ProfileWriterBlock — strips markers and body' {
    It 'removes block from CRLF fixture and preserves surrounding content' {
        $tmp = [IO.Path]::GetTempFileName()
        try {
            Copy-Item (Join-Path $script:Fixtures 'profile_crlf.sh') $tmp -Force
            Remove-ProfileWriterBlock -ProfileFile $tmp
            $content = Get-Content -Path $tmp -Raw
            $content | Should -Not -Match 'BEGIN: Claude Code Bedrock Configuration'
            $content | Should -Not -Match 'END: Claude Code Bedrock Configuration'
            $content | Should -Match 'Pre-existing user content'
            $content | Should -Match 'Trailing user content'
        } finally {
            Remove-Item $tmp -Force -ErrorAction SilentlyContinue
        }
    }
    It 'removes block from LF fixture and preserves surrounding content' {
        $tmp = [IO.Path]::GetTempFileName()
        try {
            Copy-Item (Join-Path $script:Fixtures 'profile_crlf_unix.sh') $tmp -Force
            Remove-ProfileWriterBlock -ProfileFile $tmp
            $content = Get-Content -Path $tmp -Raw
            $content | Should -Not -Match 'BEGIN: Claude Code Bedrock Configuration'
            $content | Should -Match 'Pre-existing user content'
            $content | Should -Match 'Trailing user content'
        } finally {
            Remove-Item $tmp -Force -ErrorAction SilentlyContinue
        }
    }
}
