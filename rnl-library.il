; CIW cell/library export and a sequential queue using the same native jobs.
unless(boundp('rnlActiveBatch) rnlActiveBatch = nil)
unless(boundp('rnlLastBatch) rnlLastBatch = nil)
unless(boundp('rnlCIWExportForm) rnlCIWExportForm = nil)

procedure(rnlInstallCIW()
  let((menu)
    when(boundp('ciwFileMenu) && ciwFileMenu->ExportItem
      menu = ciwFileMenu->ExportItem->hiSubMenu
      when(menu && !member('rnlCIWExportItem menu->_menuItemList)
        hiAddMenuItem(menu hiCreateMenuItem(?name 'rnlCIWExportItem
          ?itemText "RNL..." ?callback "rnlShowCIWExport()"))))
    t
  )
)

procedure(rnlCIWDefaults(form)
  let((name)
    name = if(form->exportScope->value == "Library" then
      form->sourceLibrary->value else form->sourceCell->value)
    when(name && name != ""
      form->outputDirectory->value = strcat(getWorkingDir() "/" name "_rnl"))
    t
  )
)

procedure(rnlCIWModeChanged(form)
  hiSetFieldEnabled(form->sourceCell form->exportScope->value == "Cell")
  hiSetFieldEnabled(form->includeExternal t)
  hiSetFieldEnabled(form->includeHierarchy form->exportScope->value == "Cell")
  hiSetFieldEnabled(form->fileMode form->exportScope->value == "Cell")
  unless(form->rnlPreviousScope == form->exportScope->value
    form->includeExternal->value = form->exportScope->value == "Cell"
    form->rnlPreviousScope = form->exportScope->value)
  when(form->exportScope->value == "Library"
    form->includeHierarchy->value = t
    form->fileMode->value = "Separate files")
  rnlCIWDefaults(form)
)

procedure(rnlCIWLibraryChanged(form)
  let((lib cells previous)
    previous = form->sourceCell->value
    lib = ddGetObj(form->sourceLibrary->value)
    cells = nil
    when(lib
      foreach(cell lib->cells
        when(rnlFindView(lib->name cell->name rnlViewList(form->schematicViews->value "schematicViews")) cells = cons(cell->name cells))))
    cells = sort(cells 'alphalessp)
    unless(cells cells = list(""))
    form->sourceCell->choices = cells
    form->sourceCell->value = if(member(previous cells) then previous else car(cells))
    rnlCIWModeChanged(form)
  )
)

procedure(rnlCIWExportCallback(form)
  let((format result)
    format = if(form->netlistFormat->value == "Spectre (simulation)" then "spectre" else "cdl")
    result = errset(
      if(form->exportScope->value == "Library" then
        rnlStartLibraryExport(form->sourceLibrary->value form->outputDirectory->value
          form->includeHierarchy->value form->fileMode->value == "Separate files"
          form->drawingScale->value format form->includeExternal->value t form->schematicViews->value form->symbolViews->value form->exportSelectionBox->value form->autoCheckSave->value form->exportSymbols->value)
      else
        rnlStartExport(form->sourceLibrary->value form->sourceCell->value nil
          form->outputDirectory->value form->includeHierarchy->value
          form->fileMode->value == "Separate files" form->drawingScale->value format
          if(form->includeExternal->value then "" else form->sourceLibrary->value) nil t form->schematicViews->value form->symbolViews->value form->exportSelectionBox->value form->autoCheckSave->value form->exportSymbols->value)) t)
    hiSetCallbackStatus(form if(result then t else nil))
    unless(result rnlNotice("Export could not start. See the CIW error message."))
  )
)

procedure(rnlShowCIWExport()
  let((libraries cv)
    libraries = sort(ddGetLibList()~>name 'alphalessp)
    unless(libraries error("No libraries are available.\n"))
    unless(rnlCIWExportForm && rnlCIWExportForm->exportSymbols
      rnlCIWExportForm = hiCreateAppForm(?name 'rnlCIWExportFormV094 ?formTitle "Export RNL"
        ?callback 'rnlCIWExportCallback ?buttonLayout 'OKCancel ?unmapAfterCB t ?dontBlock t
        ?fields list(
          hiCreateCyclicField(?name 'exportScope ?prompt "Export" ?choices list("Cell" "Library")
            ?value "Cell" ?callback "rnlCIWModeChanged(hiGetCurrentForm())")
          hiCreateCyclicField(?name 'sourceLibrary ?prompt "Library" ?choices libraries
            ?callback "rnlCIWLibraryChanged(hiGetCurrentForm())")
          hiCreateCyclicField(?name 'sourceCell ?prompt "Schematic cell" ?choices list("")
            ?callback "rnlCIWDefaults(hiGetCurrentForm())")
          hiCreateStringField(?name 'schematicViews ?prompt "Schematic views (highest priority first)" ?value "schematic"
            ?callback "rnlCIWLibraryChanged(hiGetCurrentForm())")
          hiCreateStringField(?name 'symbolViews ?prompt "Symbol views (highest priority first)" ?value "symbol")
          hiCreateBooleanButton(?name 'autoCheckSave ?buttonText "Automatically Check & Save stale schematics" ?value t)
          hiCreateBooleanButton(?name 'exportSelectionBox ?buttonText "Export selection box" ?value t)
          hiCreateBooleanButton(?name 'exportSymbols ?buttonText "Export symbols.rnl separately" ?value nil)
          hiCreateStringField(?name 'outputDirectory ?prompt "New output directory")
          hiCreateCyclicField(?name 'netlistFormat ?prompt "Netlist format"
            ?choices list("Spectre (simulation)" "SPICE / CDL (LVS)") ?value "Spectre (simulation)")
          hiCreateBooleanButton(?name 'includeHierarchy ?buttonText "Include subcircuits" ?value t)
          hiCreateBooleanButton(?name 'includeExternal ?buttonText "Include referenced external cells" ?value nil)
          hiCreateCyclicField(?name 'fileMode ?prompt "Subcircuits"
            ?choices list("Combined file" "Separate files") ?value "Separate files")
          hiCreateFloatField(?name 'drawingScale ?prompt "EDIF units per RNL unit" ?value 10.0))))
    rnlCIWExportForm->sourceLibrary->choices = libraries
    unless(member(rnlCIWExportForm->sourceLibrary->value libraries)
      rnlCIWExportForm->sourceLibrary->value = car(libraries))
    cv = geGetEditCellView()
    when(cv && cv->cellViewType == "schematic"
      rnlCIWExportForm->sourceLibrary->value = cv->libName
      rnlCIWExportForm->sourceCell->choices = list(cv->cellName)
      rnlCIWExportForm->sourceCell->value = cv->cellName)
    rnlCIWLibraryChanged(rnlCIWExportForm)
    hiDisplayForm(rnlCIWExportForm)
  )
)

procedure(rnlStartLibraryExport(lib destination hierarchy split scale @optional (format "spectre") (external nil) (notify t) (schematicViews "schematic") (symbolViews "symbol") (exportSelectionBox t) (autoCheckSave t) (exportSymbols nil))
  let((library tasks skipped work schematicList symbolList selected)
    when(rnlActiveJob || rnlActiveBatch error("An RNL export is already running.\n"))
    library = ddGetObj(lib)
    unless(library error("Library %s was not found.\n" lib))
    unless(destination && destination != "" && scale && scale > 0
      error("Choose a new output directory and a positive scale.\n"))
    destination = simplifyFilename(destination)
    when(isDir(destination) || isFile(destination) error("Output already exists: %s. Choose a new directory.\n" destination))
    unless(member(format list("spectre" "cdl")) error("Choose spectre or cdl.\n"))
    unless(hierarchy error("Library export includes all of its cells; use the external-cell option to control foreign definitions.\n"))
    schematicList = rnlViewList(schematicViews "schematicViews")
    symbolList = rnlViewList(symbolViews "symbolViews")
    foreach(cell library->cells
      cond(
        ((selected = rnlFindView(lib cell->name schematicList)) tasks = cons(list(cell->name selected "schematic") tasks))
        ((selected = rnlFindView(lib cell->name symbolList)) tasks = cons(list(cell->name selected "symbol") tasks))
        (t skipped = cons(cell->name skipped))))
    unless(tasks error("Library %s has no schematic or symbol views.\n" lib))
    tasks = sort(tasks lambda((a b) alphalessp(car(a) car(b))))
    work = makeTempFileName("/tmp/rnlLibraryXXXX")
    unless(createDir(work) error("Cannot create library work directory.\n"))
    rnlActiveBatch = list(nil 'library lib 'destination destination 'notify notify
      'hierarchy hierarchy 'split t 'scale scale 'format format 'external external
      'schematicViews schematicViews 'symbolViews symbolViews 'exportSelectionBox exportSelectionBox 'autoCheckSave autoCheckSave 'exportSymbols exportSymbols 'tasks tasks 'parts nil 'failures nil 'skipped skipped 'directory work 'phase "cells" 'count 0 'total length(tasks))
    rnlLastBatch = rnlActiveBatch
    printf("RNL: library %s: %d cells; work: %s\n" lib length(tasks) work)
    rnlBatchNext()
    work
  )
)

procedure(rnlStartSymbolJob(lib cell scale format @optional (view "symbol") (schematicViews "schematic") (symbolViews "symbol") (exportSelectionBox t))
  let((cv cds work port command selectionRows)
    cv = dbOpenCellViewByType(lib cell view nil "r")
    unless(cv error("Cannot open %s/%s/%s.\n" lib cell view))
    unwindProtect(
      progn(rnlCheckSavedSymbol(cv)
        when(exportSelectionBox selectionRows = list(rnlSelectionBoxRow(cv))))
      dbClose(cv))
    cds = ddGetForcedLib()
    when(!cds || cds == "" cds = ddGetStartup("cds.lib"))
    work = makeTempFileName("/tmp/rnlSymbolXXXX")
    unless(createDir(work) error("Cannot create symbol work directory.\n"))
    port = outfile(strcat(work "/selection-boxes.sx"))
    fprintf(port "%L\n" selectionRows)
    close(port)
    port = outfile(strcat(work "/request.sx"))
    fprintf(port "%L\n" list("rnl-symbol-job-v3" rnlInstallDirectory lib cell cds
      getWorkingDir() car(getInstallPath()) scale format view schematicViews symbolViews exportSelectionBox))
    close(port)
    command = strcat(rnlShellQuote(strcat(rnlInstallDirectory "/bin/rnl-virtuoso")) " " rnlShellQuote(strcat(work "/request.sx")))
    rnlActiveJob = list(nil 'directory work)
    rnlLastJob = rnlActiveJob
    rnlActiveJob->process = ipcBeginProcess(command "" 'rnlProcessOutput 'rnlProcessOutput 'rnlProcessDone)
    work
  )
)

procedure(rnlBatchRecordFailure(task stage reason logs)
  let((batch port)
    batch = rnlActiveBatch
    when(stage == "preflight"
      logs = strcat(batch->directory "/preflight_" sprintf(nil "%d" batch->count) ".log")
      port = outfile(logs)
      when(port fprintf(port "%s/%s/%s\n%s\n" batch->library car(task) cadr(task) reason) close(port)))
    batch->failures = cons(list(car(task) cadr(task) stage reason logs) batch->failures)
    printf("RNL: skipped failed cell %s/%s: %s\n" batch->library car(task) reason)
  )
)

procedure(rnlBatchNext()
  let((batch task result destination port command)
    batch = rnlActiveBatch
    while(batch->tasks && !rnlActiveJob
      task = car(batch->tasks)
      batch->currentTask = task
      batch->tasks = cdr(batch->tasks)
      batch->count = batch->count + 1
      printf("RNL: library cell %d/%d: %s/%s (%s)\n" batch->count batch->total batch->library car(task) cadr(task))
      batch->starting = t
      rnlPreflightError = nil
      result = errset(
        if(caddr(task) == "schematic" then
          destination = strcat(batch->directory "/cell_" sprintf(nil "%d" batch->count))
          rnlStartExport(batch->library car(task) cadr(task) destination batch->hierarchy t
            batch->scale batch->format if(batch->external then "" else batch->library) t t batch->schematicViews batch->symbolViews batch->exportSelectionBox batch->autoCheckSave)
        else rnlStartSymbolJob(batch->library car(task) batch->scale batch->format cadr(task) batch->schematicViews batch->symbolViews batch->exportSelectionBox)) t)
      batch->starting = nil
      unless(result
        rnlBatchRecordFailure(task "preflight" if(rnlPreflightError then rnlPreflightError else sprintf(nil "%L" errset.errset)) batch->directory))
    )
    unless(rnlActiveJob
      batch->phase = "merge"
      port = outfile(strcat(batch->directory "/request.sx"))
      fprintf(port "%L\n" list("rnl-library-merge-v3" rnlInstallDirectory batch->library
        batch->destination batch->format batch->split reverse(batch->parts) batch->external batch->hierarchy batch->skipped reverse(batch->failures) batch->exportSymbols))
      close(port)
      command = strcat(rnlShellQuote(strcat(rnlInstallDirectory "/bin/rnl-virtuoso")) " "
        rnlShellQuote(strcat(batch->directory "/request.sx")))
      rnlActiveJob = list(nil 'directory batch->directory 'destination batch->destination 'entry "library-report.json")
      rnlLastJob = rnlActiveJob
      rnlActiveJob->process = ipcBeginProcess(command "" 'rnlProcessOutput 'rnlProcessOutput 'rnlProcessDone)
    )
  )
)

procedure(rnlBatchProcessDone(job status)
  let((batch port line message)
    batch = rnlActiveBatch
    if(batch->phase == "merge" then
      batch->status = status
      rnlActiveBatch = nil
      if(status == 0 then
        message = strcat("Library export finished: " batch->destination "/library-report.json"
          sprintf(nil "\nFailed cells: %d (details in report)." length(batch->failures)))
      else
        message = "Library export failed."
        port = infile(strcat(job->directory "/error.txt"))
        when(port when(gets(line port) message = strcat(message "\n" line)) close(port)))
      message = strcat(message "\nWork: " batch->directory "\nLast cell logs: " job->directory)
      if(batch->notify then rnlNotice(message) else printf("RNL: %s\n" message))
    else
      if(status == 0 then
        batch->parts = cons(strcat(job->directory "/library-parts.json") batch->parts)
      else
        message = "Native export or conversion failed."
        port = infile(strcat(job->directory "/error.txt"))
        when(port
          message = ""
          while(gets(line port) message = strcat(message line))
          close(port))
        rnlBatchRecordFailure(batch->currentTask "export" message job->directory))
      rnlBatchNext())
  )
)
