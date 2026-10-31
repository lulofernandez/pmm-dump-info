-- Finder launcher for pmm-dump-inspect: writes the HTML report of each
-- selected pmm-dump file (next to the file) and opens it in the browser.
-- Compiled into ~/Applications/pmm-dump-inspect.app by install-finder.sh.

property inspectBin : "__BIN__"

on open theFiles
	set failed to {}
	repeat with f in theFiles
		set p to POSIX path of f
		try
			do shell script quoted form of inspectBin & " -open " & quoted form of p & " 2>&1"
		on error errMsg
			set end of failed to p & return & errMsg
		end try
	end repeat
	if failed is not {} then
		set AppleScript's text item delimiters to return & return
		set msg to failed as text
		set AppleScript's text item delimiters to ""
		display dialog msg with title "pmm-dump-inspect" buttons {"OK"} default button "OK" with icon caution
	end if
end open

on run
	set theFiles to choose file with prompt "Choose pmm-dump files to inspect:" with multiple selections allowed
	open theFiles
end run
