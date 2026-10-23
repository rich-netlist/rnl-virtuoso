; Public asynchronous API for CIW scripts and Virtuoso bridge agents.
; Internal job objects stay in Virtuoso; public results contain only plain data.
unless(boundp('rnlAPIJobs) rnlAPIJobs = makeTable('rnlAPIJobs nil))
unless(boundp('rnlLastExportId) rnlLastExportId = nil)

procedure(rnlExporterInfo()
  list(list("api_version" 1) list("tool_version" "0.9.4")
    list("install_directory" rnlInstallDirectory)
    list("busy" if(rnlActiveJob || rnlActiveBatch then t else nil))
    list("active_id" cond((rnlActiveBatch rnlActiveBatch->directory)
      (rnlActiveJob rnlActiveJob->directory) (t nil))))
)

procedure(rnlAPIString(value name)
  unless(stringp(value) && value != "" error("%s must be a nonempty string.\n" name))
  value
)

procedure(rnlAPIBoolean(value name)
  unless(value == t || value == nil error("%s must be t or nil.\n" name))
  value
)

procedure(rnlAPIOptions(lib destination format scale)
  rnlAPIString(lib "library")
  rnlAPIString(destination "destination")
  when(rnlActiveJob || rnlActiveBatch error("An RNL export is already running.\n"))
  unless(member(format list("spectre" "cdl")) error("format must be spectre or cdl.\n"))
  unless(numberp(scale) && scale > 0 error("scale must be a positive number.\n"))
  destination = simplifyFilename(destination)
  unless(substring(destination 1 1) == "/"
    destination = simplifyFilename(strcat(getWorkingDir() "/" destination)))
  when(isDir(destination) || isFile(destination)
    error("Output already exists: %s. Choose a new directory.\n" destination))
  destination
)

procedure(rnlAPITrack(id kind job)
  rnlAPIJobs[id] = list(nil 'kind kind 'job job)
  rnlLastExportId = id
  id
)

procedure(rnlExportCell(lib cell destination @key (view nil)
    (format "spectre") (hierarchy t) (external t) (fileMode "separate") (scale 10.0)
    (schematicViews "schematic") (symbolViews "symbol") (exportSelectionBox t) (autoCheckSave t) (exportSymbols nil))
  let((id)
    rnlAPIString(cell "cell")
    when(view rnlAPIString(view "view"))
    rnlAPIBoolean(hierarchy "hierarchy")
    rnlAPIBoolean(external "external")
    rnlAPIBoolean(exportSelectionBox "exportSelectionBox")
    rnlAPIBoolean(autoCheckSave "autoCheckSave")
    rnlAPIBoolean(exportSymbols "exportSymbols")
    unless(member(fileMode list("separate" "combined"))
      error("fileMode must be separate or combined.\n"))
    destination = rnlAPIOptions(lib destination format scale)
    id = rnlStartExport(lib cell view destination hierarchy fileMode == "separate"
      scale format if(external then "" else lib) nil nil schematicViews symbolViews exportSelectionBox autoCheckSave exportSymbols)
    rnlAPITrack(id "cell" rnlLastJob)
  )
)

procedure(rnlExportLibrary(lib destination @key (format "spectre") (external nil) (scale 10.0)
    (schematicViews "schematic") (symbolViews "symbol") (exportSelectionBox t) (autoCheckSave t) (exportSymbols nil))
  let((id)
    rnlAPIBoolean(external "external")
    rnlAPIBoolean(exportSelectionBox "exportSelectionBox")
    rnlAPIBoolean(autoCheckSave "autoCheckSave")
    rnlAPIBoolean(exportSymbols "exportSymbols")
    destination = rnlAPIOptions(lib destination format scale)
    id = rnlStartLibraryExport(lib destination t t scale format external nil schematicViews symbolViews exportSelectionBox autoCheckSave exportSymbols)
    rnlAPITrack(id "library" rnlLastBatch)
  )
)

procedure(rnlExportStatus(@optional (id rnlLastExportId))
  let((record job batch state phase completed total failures)
    unless(id && (record = rnlAPIJobs[id])
      error("Unknown RNL export id: %L. Use the id returned by the public API in this session.\n" id))
    job = record->job
    batch = record->kind == "library"
    state = cond((job->status == nil "running") (job->status != 0 "failed")
      (batch && job->failures "partial") (t "succeeded"))
    phase = if(batch then job->phase else "cell")
    total = if(batch then job->total else 1)
    completed = if(state != "running" || phase == "merge" then total else
      if(batch then job->count - 1 else 0))
    failures = if(batch then length(job->failures) else if(state == "failed" then 1 else 0))
    list(list("id" id) list("kind" record->kind) list("state" state)
      list("phase" phase) list("exit_code" job->status)
      list("output_directory" job->destination)
      list("report" strcat(job->destination if(batch then "/library-report.json" else "/export-report.json")))
      list("work_directory" job->directory) list("error_file" strcat(job->directory "/error.txt"))
      list("completed_cells" completed) list("total_cells" total) list("failed_cells" failures)
      list("current_cell" when(batch && state == "running" && phase == "cells" car(job->currentTask))))
  )
)
