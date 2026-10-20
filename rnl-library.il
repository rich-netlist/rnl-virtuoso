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
  hiSetFieldEnabled(form->includeExternal form->exportScope->value == "Library")
  hiSetFieldEnabled(form->includeHierarchy form->exportScope->value == "Cell")
  when(form->exportScope->value == "Library" form->includeHierarchy->value = t)
  rnlCIWDefaults(form)
)

procedure(rnlCIWLibraryChanged(form)
  let((lib cells previous)
    previous = form->sourceCell->value
    lib = ddGetObj(form->sourceLibrary->value)
    cells = nil
    when(lib
      foreach(cell lib->cells
        when(ddGetObj(lib->name cell->name "schematic") cells = cons(cell->name cells))))
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
          form->drawingScale->value format form->includeExternal->value)
      else
        rnlStartExport(form->sourceLibrary->value form->sourceCell->value "schematic"
          form->outputDirectory->value form->includeHierarchy->value
          form->fileMode->value == "Separate files" form->drawingScale->value format)) t)
    hiSetCallbackStatus(form if(result then t else nil))
    unless(result rnlNotice("Export could not start. See the CIW error message."))
  )
)

procedure(rnlShowCIWExport()
  let((libraries cv)
    libraries = sort(ddGetLibList()~>name 'alphalessp)
    unless(libraries error("No libraries are available.\n"))
    unless(rnlCIWExportForm
      rnlCIWExportForm = hiCreateAppForm(?name 'rnlCIWExportForm ?formTitle "Export RNL"
        ?callback 'rnlCIWExportCallback ?buttonLayout 'OKCancel ?unmapAfterCB t ?dontBlock t
        ?fields list(
          hiCreateCyclicField(?name 'exportScope ?prompt "Export" ?choices list("Cell" "Library")
            ?value "Cell" ?callback "rnlCIWModeChanged(hiGetCurrentForm())")
          hiCreateCyclicField(?name 'sourceLibrary ?prompt "Library" ?choices libraries
            ?callback "rnlCIWLibraryChanged(hiGetCurrentForm())")
          hiCreateCyclicField(?name 'sourceCell ?prompt "Schematic cell" ?choices list("")
            ?callback "rnlCIWDefaults(hiGetCurrentForm())")
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

procedure(rnlStartLibraryExport(lib destination hierarchy split scale @optional (format "spectre") (external nil))
  let((library tasks skipped work)
    when(rnlActiveJob || rnlActiveBatch error("An RNL export is already running.\n"))
    library = ddGetObj(lib)
    unless(library error("Library %s was not found.\n" lib))
    unless(destination && destination != "" && scale && scale > 0
      error("Choose a new output directory and a positive scale.\n"))
    destination = simplifyFilename(destination)
    when(isDir(destination) || isFile(destination) error("Output already exists: %s. Choose a new directory.\n" destination))
    unless(member(format list("spectre" "cdl")) error("Choose spectre or cdl.\n"))
    unless(hierarchy error("Library export includes all of its cells; use the external-cell option to control foreign definitions.\n"))
    foreach(cell library->cells
      cond(
        (ddGetObj(lib cell->name "schematic") tasks = cons(list(cell->name "schematic") tasks))
        (ddGetObj(lib cell->name "symbol") tasks = cons(list(cell->name "symbol") tasks))
        (t skipped = cons(cell->name skipped))))
    unless(tasks error("Library %s has no schematic or symbol views.\n" lib))
    tasks = sort(tasks lambda((a b) alphalessp(car(a) car(b))))
    work = makeTempFileName("/tmp/rnlLibraryXXXX")
    unless(createDir(work) error("Cannot create library work directory.\n"))
    rnlActiveBatch = list(nil 'library lib 'destination destination
      'hierarchy hierarchy 'split split 'scale scale 'format format 'external external
      'tasks tasks 'parts nil 'skipped skipped 'directory work 'phase "cells" 'count 0 'total length(tasks))
    rnlLastBatch = rnlActiveBatch
    printf("RNL: library %s: %d cells; work: %s\n" lib length(tasks) work)
    rnlBatchNext()
    work
  )
)

procedure(rnlStartSymbolJob(lib cell scale format)
  let((cv cds work port command)
    cv = dbOpenCellViewByType(lib cell "symbol" nil "r")
    unless(cv error("Cannot open %s/%s/symbol.\n" lib cell))
    unwindProtect(
      when(cv->modifiedButNotSaved error("Save %s/%s/symbol before exporting.\n" lib cell))
      dbClose(cv))
    cds = ddGetForcedLib()
    when(!cds || cds == "" cds = ddGetStartup("cds.lib"))
    work = makeTempFileName("/tmp/rnlSymbolXXXX")
    unless(createDir(work) error("Cannot create symbol work directory.\n"))
    port = outfile(strcat(work "/request.sx"))
    fprintf(port "%L\n" list("rnl-symbol-job-v1" rnlInstallDirectory lib cell cds
      getWorkingDir() car(getInstallPath()) scale format))
    close(port)
    command = strcat(rnlShellQuote(strcat(rnlInstallDirectory "/bin/rnl-virtuoso")) " " rnlShellQuote(strcat(work "/request.sx")))
    rnlActiveJob = list(nil 'directory work)
    rnlLastJob = rnlActiveJob
    rnlActiveJob->process = ipcBeginProcess(command "" 'rnlProcessOutput 'rnlProcessOutput 'rnlProcessDone)
    work
  )
)

procedure(rnlBatchNext()
  let((batch task result destination port command)
    batch = rnlActiveBatch
    if(batch->tasks then
      task = car(batch->tasks)
      batch->tasks = cdr(batch->tasks)
      batch->count = batch->count + 1
      printf("RNL: library cell %d/%d: %s/%s (%s)\n" batch->count batch->total batch->library car(task) cadr(task))
      batch->starting = t
      result = errset(
        if(cadr(task) == "schematic" then
          destination = strcat(batch->directory "/cell_" sprintf(nil "%d" batch->count))
          rnlStartExport(batch->library car(task) "schematic" destination batch->hierarchy t
            batch->scale batch->format if(batch->external then "" else batch->library) t)
        else rnlStartSymbolJob(batch->library car(task) batch->scale batch->format)) t)
      batch->starting = nil
      unless(result
        batch->status = 1
        rnlActiveBatch = nil
        rnlNotice(strcat("Library export could not start a cell. See CIW.\nWork: " batch->directory)))
    else
      batch->phase = "merge"
      port = outfile(strcat(batch->directory "/request.sx"))
      fprintf(port "%L\n" list("rnl-library-merge-v1" rnlInstallDirectory batch->library
        batch->destination batch->format batch->split reverse(batch->parts) batch->external batch->hierarchy batch->skipped))
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
    if(status != 0 || batch->phase == "merge" then
      batch->status = status
      rnlActiveBatch = nil
      if(status == 0 then
        message = strcat("Library export complete: " batch->destination "/library-report.json")
      else
        message = "Library export failed."
        port = infile(strcat(job->directory "/error.txt"))
        when(port when(gets(line port) message = strcat(message "\n" line)) close(port)))
      rnlNotice(strcat(message "\nWork: " batch->directory "\nLast cell logs: " job->directory))
    else
      batch->parts = cons(strcat(job->directory "/library-parts.json") batch->parts)
      rnlBatchNext())
  )
)
