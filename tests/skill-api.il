; Run rnlTestPublicAPI() in an idle Virtuoso with the plugin loaded.
; No libraries are created or changed. Synthetic jobs test status transitions;
; real native export/IPC completion is checked separately through the bridge.
procedure(rnlTestAPIAssert(condition message)
  unless(condition error("RNL API test: %s\n" message))
)

procedure(rnlTestAPIValue(key status)
  cadr(assoc(key status))
)

procedure(rnlTestPublicAPI()
  unless(!rnlActiveJob && !rnlActiveBatch error("Run API tests only when idle.\n"))
  let((rnlAPIJobs rnlLastExportId job id status batch)
    rnlAPIJobs = makeTable('rnlTestAPIJobs nil)
    rnlLastExportId = nil
    rnlTestAPIAssert(rnlTestAPIValue("api_version" rnlExporterInfo()) == 1 "API version")
    foreach(expression list(
        'rnlExportStatus("missing")
        'rnlExportStatus()
        'rnlExportCell("" "cell" "/tmp/unused")
        'rnlExportCell("lib" "" "/tmp/unused")
        'rnlExportCell("lib" "cell" "")
        'rnlExportCell("lib" "cell" "/tmp/unused" ?format "invalid")
        'rnlExportCell("lib" "cell" "/tmp/unused" ?fileMode "invalid")
        'rnlExportCell("lib" "cell" "/tmp/unused" ?scale 0)
        'rnlExportCell("lib" "cell" "/tmp/unused" ?scale "10")
        'rnlExportCell("lib" "cell" "/tmp/unused" ?hierarchy "nil")
        'rnlExportCell("lib" "cell" "/tmp/unused" ?external 0)
        'rnlExportCell("lib" "cell" "/tmp/unused" ?exportSelectionBox "false")
        'rnlExportLibrary("lib" "/tmp/unused" ?exportSelectionBox 0)
        'rnlExportCell("lib" "cell" "/tmp/unused" ?autoCheckSave "false")
        'rnlExportLibrary("lib" "/tmp/unused" ?autoCheckSave 0)
        'rnlExportCell("lib" "cell" "/tmp/unused" ?exportSymbols "false")
        'rnlExportLibrary("lib" "/tmp/unused" ?exportSymbols 0)
        'rnlExportCell("lib" "cell" "/tmp/unused" ?schematicViews "")
        'rnlExportCell("lib" "cell" "/tmp/unused" ?symbolViews "../symbol")
        'rnlExportCell("lib" "cell" "/tmp")
        'rnlExportLibrary("lib" "/tmp/unused" ?external "false")
        'rnlExportLibrary("lib" "/tmp"))
      rnlTestAPIAssert(!errset(eval(expression) nil) sprintf(nil "accepted invalid request %L" expression)))
    rnlTestAPIAssert(!rnlLastExportId && !rnlActiveJob && !rnlActiveBatch "invalid starts left a job")
    rnlTestAPIAssert(equal(rnlViewList(" schematic_ideal  schematic schematic_ideal " "test")
      list("schematic_ideal" "schematic")) "view order or deduplication")

    rnlTestTerminalInterfaces()
    id = "/tmp/rnl-api-test-cell"
    job = list(nil 'directory id 'destination "/tmp/result")
    rnlAPITrack(id "cell" job)
    status = rnlExportStatus()
    rnlTestAPIAssert(rnlTestAPIValue("state" status) == "running" "new job state")
    rnlTestAPIAssert(rnlTestAPIValue("completed_cells" status) == 0 "new job progress")
    job->status = 0
    rnlTestAPIAssert(rnlTestAPIValue("state" rnlExportStatus(id)) == "succeeded" "zero status")
    job->status = 2
    rnlTestAPIAssert(rnlTestAPIValue("state" rnlExportStatus(id)) == "failed" "nonzero status")
    rnlTestAPIAssert(rnlTestAPIValue("failed_cells" rnlExportStatus(id)) == 1 "failed count")

    batch = list(nil 'directory "/tmp/rnl-api-test-library" 'destination "/tmp/library"
      'phase "cells" 'count 2 'total 3 'currentTask list("cell2" "schematic"))
    rnlAPITrack(batch->directory "library" batch)
    status = rnlExportStatus()
    rnlTestAPIAssert(rnlTestAPIValue("completed_cells" status) == 1 "batch progress")
    rnlTestAPIAssert(rnlTestAPIValue("current_cell" status) == "cell2" "batch current cell")
    batch->phase = "merge"
    rnlTestAPIAssert(rnlTestAPIValue("completed_cells" rnlExportStatus()) == 3 "merge progress")
    rnlTestAPIAssert(!rnlTestAPIValue("current_cell" rnlExportStatus()) "merge has no current cell")
    batch->status = 0
    rnlTestAPIAssert(rnlTestAPIValue("state" rnlExportStatus()) == "succeeded" "batch success")
    batch->failures = list(list("cell1" "schematic" "export" "reason" "/tmp/log"))
    rnlTestAPIAssert(rnlTestAPIValue("state" rnlExportStatus()) == "partial" "partial is not success")
    batch->status = 2
    rnlTestAPIAssert(rnlTestAPIValue("state" rnlExportStatus()) == "failed" "merge failure wins")
    rnlTestAPIAssert(rnlTestAPIValue("id" rnlExportStatus(id)) == id "prior job retained")
    let((rnlActiveJob)
      rnlActiveJob = job
      rnlTestAPIAssert(rnlTestAPIValue("busy" rnlExporterInfo()) "busy info")
      rnlTestAPIAssert(!errset(rnlExportCell("lib" "cell" "/tmp/unused") nil) "concurrent cell allowed")
      rnlTestAPIAssert(!errset(rnlExportLibrary("lib" "/tmp/unused") nil) "concurrent library allowed"))
    t
  )
)

; No source cellviews are opened or saved. Use Virtuoso's member-name parser.
procedure(rnlTestTerminalInterfaces()
  let((names before)
    names = list("FRAME<0:15>" "REPET<1:17>" "GND")
    before = copy(names)
    rnlTestAPIAssert(!rnlMissingTerminals(names list("GND" "REPET<17:1>" "FRAME<15:0>"))
      "reversed bus range rejected")
    rnlTestAPIAssert(equal(names before) "comparison mutated source order")
    rnlTestAPIAssert(!rnlMissingTerminals(list("A<0:3>") list("A<3>" "A<1>" "A<0>" "A<2>"))
      "equivalent split interface rejected")
    rnlTestAPIAssert(equal(rnlMissingTerminals(list("A<3:0>") list("A<2:0>")) list("A<3>"))
      "missing bus bit accepted")
    rnlTestAPIAssert(equal(rnlMissingTerminals(list("P") list("p")) list("P"))
      "terminal case folded")
    t
  )
)
