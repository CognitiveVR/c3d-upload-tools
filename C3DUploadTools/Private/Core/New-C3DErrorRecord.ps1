function New-C3DErrorRecord {
    <#
    .SYNOPSIS
        Creates standardized PowerShell error records with proper categories and context.

    .DESCRIPTION
        Helper function to create consistent error records across the C3D module with
        appropriate error categories, target objects, and recommended actions.

    .PARAMETER Message
        The error message to display.

    .PARAMETER ErrorId
        A unique identifier for the error type.

    .PARAMETER Category
        The PowerShell error category that best describes the error.

    .PARAMETER TargetObject
        The object that was being processed when the error occurred.

    .PARAMETER InnerException
        An underlying exception that caused this error.

    .PARAMETER RecommendedAction
        Suggested action for the user to resolve the error.

    .EXAMPLE
        $errorRecord = New-C3DErrorRecord -Message "Invalid API key" -ErrorId "InvalidApiKey" -Category AuthenticationError -TargetObject $apiKey
        $PSCmdlet.ThrowTerminatingError($errorRecord)

    .OUTPUTS
        System.Management.Automation.ErrorRecord
    #>

    [CmdletBinding()]
    [OutputType([System.Management.Automation.ErrorRecord])]
    param(
        [Parameter(Mandatory)]
        [string]$Message,

        [Parameter(Mandatory)]
        [string]$ErrorId,

        [Parameter(Mandatory)]
        [System.Management.Automation.ErrorCategory]$Category,

        [Parameter()]
        [object]$TargetObject,

        [Parameter()]
        [System.Exception]$InnerException,

        [Parameter()]
        [string]$RecommendedAction
    )

    # Create the base exception
    $exception = if ($InnerException) {
        New-Object System.Exception($Message, $InnerException)
    } else {
        New-Object System.Exception($Message)
    }

    # Create the error record
    $errorRecord = New-Object System.Management.Automation.ErrorRecord(
        $exception,
        $ErrorId,
        $Category,
        $TargetObject
    )

    # Add recommended action if provided
    if ($RecommendedAction) {
        $errorRecord.ErrorDetails = New-Object System.Management.Automation.ErrorDetails($Message)
        $errorRecord.ErrorDetails.RecommendedAction = $RecommendedAction
    }

    return $errorRecord
}

function Invoke-C3DWithErrorHandling {
    <#
    .SYNOPSIS
        Wraps PowerShell operations with standardized error handling and cleanup.

    .DESCRIPTION
        Provides consistent try-catch-finally pattern with proper error categorization,
        resource cleanup, and detailed error logging for C3D operations.

    .PARAMETER ScriptBlock
        The script block to execute with error handling.

    .PARAMETER ErrorContext
        Descriptive context for what operation is being performed.

    .PARAMETER CleanupScriptBlock
        Optional cleanup script block to run in finally block.

    .PARAMETER RetryCount
        Number of times to retry on transient errors (default: 0).

    .PARAMETER RetryDelaySeconds
        Delay between retries in seconds (default: 1).

    .EXAMPLE
        $result = Invoke-C3DWithErrorHandling -ScriptBlock {
            Upload-FileToAPI -Path $filePath
        } -ErrorContext "uploading scene file" -CleanupScriptBlock {
            Remove-TempFile $tempFile
        }

    .OUTPUTS
        Object - The result of the script block execution
    #>

    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [scriptblock]$ScriptBlock,

        [Parameter(Mandatory)]
        [string]$ErrorContext,

        [Parameter()]
        [scriptblock]$CleanupScriptBlock,

        [Parameter()]
        [int]$RetryCount = 0,

        [Parameter()]
        [int]$RetryDelaySeconds = 1
    )

    $attempt = 0
    $maxAttempts = $RetryCount + 1

    while ($attempt -lt $maxAttempts) {
        $attempt++

        try {
            Write-C3DLog -Message "Executing: $ErrorContext (attempt $attempt/$maxAttempts)" -Level Debug

            # Execute the main script block
            $result = & $ScriptBlock

            # If we get here, operation succeeded
            return $result

        } catch [System.Net.WebException] {
            $webException = $_.Exception
            $statusCode = $null

            if ($webException.Response) {
                $statusCode = [int]$webException.Response.StatusCode
            }

            Write-C3DLog -Message "Network error while $ErrorContext`: $($webException.Message)" -Level Error

            # Determine if this is a retryable error
            $isRetryable = $false
            if ($statusCode) {
                switch ($statusCode) {
                    408 { $isRetryable = $true }  # Request Timeout
                    429 { $isRetryable = $true }  # Too Many Requests
                    500 { $isRetryable = $true }  # Internal Server Error
                    502 { $isRetryable = $true }  # Bad Gateway
                    503 { $isRetryable = $true }  # Service Unavailable
                    504 { $isRetryable = $true }  # Gateway Timeout
                }
            }

            # Retry logic for transient errors
            if ($isRetryable -and $attempt -lt $maxAttempts) {
                Write-C3DLog -Message "Retryable error (HTTP $statusCode), waiting $RetryDelaySeconds seconds before retry $($attempt + 1)/$maxAttempts" -Level Warn
                Start-Sleep -Seconds $RetryDelaySeconds
                continue
            }

            # Create appropriate error record based on status code
            $errorCategory = switch ($statusCode) {
                401 { [System.Management.Automation.ErrorCategory]::AuthenticationError }
                403 { [System.Management.Automation.ErrorCategory]::PermissionDenied }
                404 { [System.Management.Automation.ErrorCategory]::ObjectNotFound }
                429 { [System.Management.Automation.ErrorCategory]::LimitsExceeded }
                default { [System.Management.Automation.ErrorCategory]::ConnectionError }
            }

            $recommendedAction = switch ($statusCode) {
                401 { "Check your C3D_DEVELOPER_API_KEY environment variable and ensure it's valid" }
                403 { "Verify your API key has the required permissions for this operation" }
                404 { "Verify the scene ID or resource exists and the URL is correct" }
                429 { "Wait before retrying the request due to rate limiting" }
                default { "Check network connectivity and try again" }
            }

            $errorRecord = New-C3DErrorRecord -Message "Failed $ErrorContext`: $($webException.Message)" -ErrorId "NetworkError_$statusCode" -Category $errorCategory -TargetObject $webException.Response -InnerException $webException -RecommendedAction $recommendedAction

            throw $errorRecord

        } catch [System.IO.IOException] {
            Write-C3DLog -Message "File I/O error while $ErrorContext`: $($_.Exception.Message)" -Level Error

            $errorRecord = New-C3DErrorRecord -Message "File operation failed while $ErrorContext`: $($_.Exception.Message)" -ErrorId "FileIOError" -Category ([System.Management.Automation.ErrorCategory]::ReadError) -TargetObject $_.Exception.FileName -InnerException $_.Exception -RecommendedAction "Check file permissions and disk space"

            throw $errorRecord

        } catch [System.ArgumentException] {
            Write-C3DLog -Message "Invalid argument while $ErrorContext`: $($_.Exception.Message)" -Level Error

            $errorRecord = New-C3DErrorRecord -Message "Invalid parameter while $ErrorContext`: $($_.Exception.Message)" -ErrorId "InvalidArgument" -Category ([System.Management.Automation.ErrorCategory]::InvalidArgument) -TargetObject $_.Exception.ParamName -InnerException $_.Exception -RecommendedAction "Check parameter values and format"

            throw $errorRecord

        } catch [System.UnauthorizedAccessException] {
            Write-C3DLog -Message "Access denied while $ErrorContext`: $($_.Exception.Message)" -Level Error

            $errorRecord = New-C3DErrorRecord -Message "Access denied while $ErrorContext`: $($_.Exception.Message)" -ErrorId "AccessDenied" -Category ([System.Management.Automation.ErrorCategory]::PermissionDenied) -TargetObject $_.TargetSite -InnerException $_.Exception -RecommendedAction "Check file permissions or run with elevated privileges"

            throw $errorRecord

        } catch {
            Write-C3DLog -Message "Unexpected error while $ErrorContext`: $($_.Exception.Message)" -Level Error
            Write-C3DLog -Message "Error type: $($_.Exception.GetType().FullName)" -Level Debug

            # Don't retry for unknown errors
            $errorRecord = New-C3DErrorRecord -Message "Unexpected error while $ErrorContext`: $($_.Exception.Message)" -ErrorId "UnexpectedError" -Category ([System.Management.Automation.ErrorCategory]::NotSpecified) -TargetObject $_.TargetObject -InnerException $_.Exception -RecommendedAction "Review error details and contact support if the issue persists"

            throw $errorRecord
        } finally {
            # Always run cleanup if provided
            if ($CleanupScriptBlock) {
                try {
                    Write-C3DLog -Message "Running cleanup after $ErrorContext" -Level Debug
                    & $CleanupScriptBlock
                } catch {
                    Write-C3DLog -Message "Error during cleanup: $($_.Exception.Message)" -Level Warn
                }
            }
        }
    }
}