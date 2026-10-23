; RNL schematic export. Loading this file installs the menu automatically.
; Stale schematic connectivity is checked and saved by default before export.
unless(boundp('rnlActiveJob) rnlActiveJob = nil)
unless(boundp('rnlMenu) rnlMenu = nil)
unless(boundp('rnlExportForm) rnlExportForm = nil)
unless(boundp('rnlPreflightError) rnlPreflightError = nil)

procedure(rnlNotice(message)
  printf("RNL: %s\n" message)
  hiDisplayAppDBox(?name 'rnlNoticeBox ?dboxBanner "RNL Export"
    ?dboxText message ?dialogStyle 'modeless ?buttonLayout 'Close
    ?dialogType hicInformationDialog)
)

procedure(rnlShellQuote(value)
  let((out)
    out = "'"
    foreach(ch parseString(value "")
      out = strcat(out if(ch == "'" then "'\"'\"'" else ch)))
    strcat(out "'")
  )
)

; dbClose in unwindProtect can clear errset.errset. Keep source diagnostics
; explicitly so the library report retains the original failure reason.
procedure(rnlSourceError(message)
  rnlPreflightError = message
  error("%s\n" message)
)

procedure(rnlTerminalMembers(names)
  let((bits members)
    foreach(name names
      members = dbProduceMemName(name)
      unless(members rnlSourceError(sprintf(nil "Cannot expand terminal name %s.\n" name)))
      foreach(bit members unless(member(bit bits) bits = cons(bit bits))))
    reverse(bits)
  )
)

; Compare electrical member names, not the direction/grouping of drawn buses.
; This never changes either view's pin order or the native netlist interface.
procedure(rnlMissingTerminals(names otherNames)
  let((other)
    other = rnlTerminalMembers(otherNames)
    setof(bit rnlTerminalMembers(names) !member(bit other))
  )
)

; A parent can be marked current even after a master adds/removes a terminal.
procedure(rnlStaleInstances(cv)
  setof(inst cv->instances
    !inst->pin && inst->master && !member(inst->master->nlAction list("ignore")) &&
    (rnlMissingTerminals(inst->instTerms~>name inst->master->terminals~>name) ||
     rnlMissingTerminals(inst->master->terminals~>name inst->instTerms~>name)))
)

procedure(rnlCheckSaved(cv @optional (autoCheckSave t))
  let((stale originalMode checkResult saveResult checkoutResult)
    stale = !dbIsConnCurrent(cv) || rnlStaleInstances(cv)
    when(stale && autoCheckSave
      originalMode = cv->mode
      printf("RNL: automatically Check & Save %s/%s/%s (stale connectivity).\n"
        cv->libName cv->cellName cv->viewName)
      ; Batch override prevents the DM/SOS Auto Checkout dialog from blocking CIW.
      checkoutResult = errset(ddAutoCheckout(list(ddGetObj(cv->libName cv->cellName cv->viewName)) t) t)
      unless(checkoutResult && car(checkoutResult) && !member(nil car(checkoutResult))
        rnlSourceError(sprintf(nil "Automatic Check & Save cannot check out %s/%s/%s. Check source-control ownership and write permissions."
          cv->libName cv->cellName cv->viewName)))
      unless(originalMode != "r" || dbReopen(cv "a")
        rnlSourceError(sprintf(nil "Automatic Check & Save cannot acquire write access to %s/%s/%s (locked or read-only)."
          cv->libName cv->cellName cv->viewName)))
      unwindProtect(
        progn(
          checkResult = errset(schCheck(cv) t)
          unless(checkResult
            rnlSourceError(sprintf(nil "Automatic Check & Save failed during schCheck for %s/%s/%s. See the CIW schematic diagnostics."
              cv->libName cv->cellName cv->viewName)))
          saveResult = errset(dbSave(cv) t)
          unless(saveResult && car(saveResult)
            rnlSourceError(sprintf(nil "Automatic Check & Save failed to save %s/%s/%s. Check write permissions and the CIW diagnostics."
              cv->libName cv->cellName cv->viewName)))
          when(!dbIsConnCurrent(cv) || rnlStaleInstances(cv)
            rnlSourceError(sprintf(nil "Automatic Check & Save did not resolve connectivity in %s/%s/%s. See the CIW errors; missing masters and incompatible interfaces must be corrected in the source design."
              cv->libName cv->cellName cv->viewName)))
          printf("RNL: checked and saved %s/%s/%s.\n" cv->libName cv->cellName cv->viewName))
        ; OA discards unsaved edits on a->r. Restore only after a successful save.
        when(originalMode == "r" && !cv->modifiedButNotSaved && cv->mode != "r"
          dbReopen(cv "r")))
    )
    when(cv->modifiedButNotSaved || !dbIsConnCurrent(cv) || rnlStaleInstances(cv)
      rnlSourceError(sprintf(nil "Check and Save %s/%s/%s before exporting%s."
        cv->libName cv->cellName cv->viewName
        if(autoCheckSave then " (unsaved changes remain)" else " (automatic Check & Save is disabled)"))))
  )
)

procedure(rnlCheckSavedSymbol(cv)
  let((source)
    ; Generated PCell submasters are transient and cannot be saved by users.
    source = if(cv->superMaster then cv->superMaster else cv)
    when(source->modifiedButNotSaved
      rnlSourceError(sprintf(nil "Save symbol %s/%s/%s before exporting."
        source->libName source->cellName source->viewName)))
  )
)

; EDIF Out omits schematic note shapes on the text layer. Snapshot only these
; non-electrical figures, in user units with the cellview's database scale.
procedure(rnlCollectAnnotations(cv annotations)
  let((figures points)
    foreach(shape cv->shapes
      when(shape->layerName == "text" && !shape->net && !shape->pin && !member(shape->objType list("label" "textDisplay"))
        points = if(member(shape->objType list("rect" "ellipse")) then shape->bBox else shape->points)
        when(shape->objType == "arc" points = shape->ellipseBBox)
        figures = cons(list(shape->objType points
          when(shape->objType == "arc" list(shape->startAngle shape->stopAngle))
          if(shape->purpose == "drawing1" then "dashed" else "solid")) figures)))
    annotations[list(cv->libName cv->cellName)] = list(cv->libName cv->cellName cv->DBUPerUU reverse(figures))
  )
)

; Preserve every source selection shape; EDIF Out can drop all but one.
procedure(rnlSelectionBoxRow(cv)
  list(cv->libName cv->cellName cv->viewName cv->DBUPerUU
    foreach(mapcar shape setof(s cv->shapes
      s->layerName == "instance" && s->purpose == "drawing" &&
      !member(s->objType list("label" "textDisplay"))) shape->bBox))
)

procedure(rnlCollectSelectionBoxes(cv snapshots)
  let((key)
    key = list(cv->libName cv->cellName)
    unless(snapshots[key] snapshots[key] = rnlSelectionBoxRow(cv)))
)

; Ordered, whitespace-separated view lists are shared by GUI and API.
procedure(rnlViewList(value label)
  let((views)
    unless(stringp(value) error("%s must be a space-separated string.\n" label))
    foreach(v parseString(value " \t\n\r")
      unless(rexMatchp("^[a-zA-Z0-9_][a-zA-Z0-9_.-]*$" v)
        error("Invalid view name in %s: %s.\n" label v))
      unless(member(v views) views = append(views list(v))))
    unless(views error("%s must contain at least one view.\n" label))
    views
  )
)

procedure(rnlFindView(lib cell views)
  car(setof(v views ddGetObj(lib cell v)))
)

procedure(rnlCheckInterface(schematic symbol)
  let((missing extra)
    missing = rnlMissingTerminals(schematic->terminals~>name symbol->terminals~>name)
    extra = rnlMissingTerminals(symbol->terminals~>name schematic->terminals~>name)
    when(missing || extra
      rnlSourceError(sprintf(nil "Terminal interface mismatch in %s/%s: schematic view %s, symbol view %s; missing from symbol %L; missing from schematic %L. Synchronize the source interfaces before exporting."
        schematic->libName schematic->cellName schematic->viewName symbol->viewName missing extra)))
  )
)

procedure(rnlCollectOrders(cv seen orders @optional (simulator "spectre") (scope "") (annotations nil) (schematicViews list("schematic")) (symbolViews list("symbol")) (symbolSelections nil) (selectionBoxes nil) (autoCheckSave t))
  let((key master cdf sim child result childView symbolView selectedSymbol)
    key = list(cv->libName cv->cellName cv->viewName)
    unless(seen[key]
      seen[key] = t
      rnlCheckSaved(cv autoCheckSave)
      ; Prepare descendants before capturing this level. Refresh the parent again
      ; afterwards if a child's Check & Save invalidated its placed interface.
      foreach(inst cv->instances
        master = inst->master
        unless(master
          rnlSourceError(sprintf(nil "Missing placed master for %s/%s/%s instance %s: %s/%s/%s. Check the reference library in cds.lib."
            cv->libName cv->cellName cv->viewName inst->name inst->libName inst->cellName inst->viewName)))
        unless(member(master->nlAction list("ignore" "stop")) ||
          (simulator != "spectre" && ddGetObj(master->libName master->cellName simulator))
          childView = rnlFindView(master->libName master->cellName schematicViews)
          when(childView
            child = dbOpenCellViewByType(master->libName master->cellName childView nil "r")
            when(child
              unwindProtect(
                unless(member(child->nlAction list("ignore" "stop"))
                  rnlCollectOrders(child seen orders simulator scope annotations schematicViews symbolViews symbolSelections selectionBoxes autoCheckSave))
                dbClose(child))))))
      rnlCheckSaved(cv autoCheckSave)
      symbolView = rnlFindView(cv->libName cv->cellName symbolViews)
      when(symbolView
        selectedSymbol = dbOpenCellViewByType(cv->libName cv->cellName symbolView nil "r")
        when(selectedSymbol
          unwindProtect(progn(rnlCheckSavedSymbol(selectedSymbol) rnlCheckInterface(cv selectedSymbol)
            when(selectionBoxes rnlCollectSelectionBoxes(selectedSymbol selectionBoxes))) dbClose(selectedSymbol))))
      when(annotations rnlCollectAnnotations(cv annotations))
      ; The selected cell's interface is needed when wrapping it as a subckt.
      cdf = cdfGetBaseCellCDF(ddGetObj(cv->libName cv->cellName))
      sim = when(cdf get(cdf->simInfo concat(simulator)))
      orders[list(cv->libName cv->cellName)] = list(cv->libName cv->cellName
        when(sim foreach(mapcar terminal sim->termOrder
          if(stringp(terminal) then terminal else get_pname(terminal)))))
      foreach(inst cv->instances
        master = inst->master
        unless(master
          rnlSourceError(sprintf(nil "Missing placed master for %s/%s/%s instance %s: %s/%s/%s. Check the reference library in cds.lib."
            cv->libName cv->cellName cv->viewName inst->name inst->libName inst->cellName inst->viewName)))
        rnlCheckSavedSymbol(master)
        symbolView = rnlFindView(master->libName master->cellName symbolViews)
        when(symbolView && symbolView != master->viewName &&
          (!symbolSelections || !symbolSelections[list(master->libName master->cellName)])
          selectedSymbol = dbOpenCellViewByType(master->libName master->cellName symbolView nil "r")
          unless(selectedSymbol error("Cannot open %s/%s/%s.\n" master->libName master->cellName symbolView))
          unwindProtect(progn(rnlCheckSavedSymbol(selectedSymbol)
            when(selectionBoxes rnlCollectSelectionBoxes(selectedSymbol selectionBoxes))) dbClose(selectedSymbol)))
        when(selectionBoxes && (!symbolView || symbolView == master->viewName)
          rnlCollectSelectionBoxes(master selectionBoxes))
        when(symbolSelections
          symbolSelections[list(master->libName master->cellName)] = list(master->libName master->cellName
            if(symbolView then symbolView else master->viewName) master->viewName))
        key = list(master->libName master->cellName)
        unless(orders[key]
          cdf = cdfGetBaseCellCDF(ddGetObj(master->libName master->cellName))
          sim = when(cdf get(cdf->simInfo concat(simulator)))
          orders[key] = list(master->libName master->cellName
            when(sim foreach(mapcar terminal sim->termOrder
              if(stringp(terminal) then terminal else get_pname(terminal)))))
        )

      )
    )
    foreach(k orders result = cons(orders[k] result))
    result
  )
)

procedure(rnlStartExport(lib cell view destination hierarchy split scale @optional (format "spectre") (scope "") (capture nil) (notify t) (schematicViews "schematic") (symbolViews "symbol") (exportSelectionBox t) (autoCheckSave t) (exportSymbols nil))
  let((cv rows work port cds command simulator hasSymbol symbol annotations annotationRows schematicList symbolList symbolView symbolSelections symbolRows selectionBoxes selectionRows)
    rnlPreflightError = nil
    when(rnlActiveJob error("An RNL export is already running.\n"))
    when(boundp('rnlActiveBatch) && rnlActiveBatch && !rnlActiveBatch->starting
      error("An RNL library export is already running.\n"))
    unless(destination && destination != "" error("Choose a new output directory.\n"))
    unless(scale && scale > 0 error("Drawing scale must be positive.\n"))
    unless(member(format list("spectre" "cdl")) error("Choose spectre or cdl.\n"))
    schematicList = rnlViewList(schematicViews "schematicViews")
    symbolList = rnlViewList(symbolViews "symbolViews")
    unless(view view = rnlFindView(lib cell schematicList))
    unless(view error("No schematic view for %s/%s in %L.\n" lib cell schematicList))
    symbolSelections = makeTable('rnlSymbolSelections nil)
    selectionBoxes = when(exportSelectionBox makeTable('rnlSelectionBoxes nil))
    simulator = if(format == "spectre" then "spectre" else "auCdl")
    cv = dbOpenCellViewByType(lib cell view nil "r")
    unless(cv error("Cannot open %s/%s/%s.\n" lib cell view))
    annotations = makeTable('rnlAnnotations nil)
    unwindProtect(
      rows = rnlCollectOrders(cv makeTable('rnlSeen nil) makeTable('rnlOrders nil) simulator scope annotations schematicList symbolList symbolSelections selectionBoxes autoCheckSave)
      dbClose(cv))
    symbolView = rnlFindView(lib cell symbolList)
    hasSymbol = symbolView != nil
    when(hasSymbol
      symbol = dbOpenCellViewByType(lib cell symbolView nil "r")
      unless(symbol error("Cannot open the current cell's symbol view.\n"))
      unwindProtect(
        progn(rnlCheckSavedSymbol(symbol)
          when(selectionBoxes rnlCollectSelectionBoxes(symbol selectionBoxes)))
        dbClose(symbol)))
    cds = ddGetForcedLib()
    when(!cds || cds == "" cds = ddGetStartup("cds.lib"))
    unless(cds error("No active cds.lib was found.\n"))
    work = makeTempFileName("/tmp/rnlXXXX")
    unless(createDir(work) error("Cannot create export work directory.\n"))
    unless(simInitEnvWithArgs(strcat(work "/si") lib cell view simulator nil)
      error("Could not initialize the native netlist exporter.\n"))
    port = outfile(strcat(work "/si/si.env") "a")
    unless(port error("Cannot write native netlist settings.\n"))
    fprintf(port "\nsimNotIncremental = 't\nsimReNetlistAll = 't\n")
    fprintf(port "simNetlistHier = 't\n")
    if(format == "spectre" then
      ; Export the schematic hierarchy even when a cell also has a stop view.
      ; Primitives and behavioral cells without a schematic retain their models.
      fprintf(port "simViewList = '%L\nsimStopList = '(\"spectre\" \"veriloga\" \"veriloga_ans\")\n" append(schematicList list("spectre" "veriloga" "veriloga_ans")))
    else
      ; Same preservation options as File > Export > CDL, Analog mode.
      fprintf(port "simViewList = '%L\nsimStopList = '(\"auCdl\")\n" cons("auCdl" schematicList))
      fprintf(port "hnlNetlistFileName = \"netlist\"\nresistorModel = \"\"\nshortRES = 2000.0\n")
      fprintf(port "preserveRES = 't\ncheckRESVAL = 't\ncheckRESSIZE = 'nil\n")
      fprintf(port "preserveCAP = 't\ncheckCAPVAL = 't\ncheckCAPAREA = 'nil\n")
      fprintf(port "preserveDIO = 't\ncheckDIOAREA = 't\ncheckDIOPERI = 't\ncheckCAPPERI = 'nil\n")
      fprintf(port "simPrintInhConnAttributes = 'nil\ncheckScale = \"meter\"\ncheckLDD = 'nil\npinMAP = 'nil\n")
      fprintf(port "preserveBangInNetlist = 'nil\nshrinkFACTOR = 0.0\nglobalPowerSig = \"\"\nglobalGndSig = \"\"\n")
      fprintf(port "displayPININFO = 't\npreserveALL = 't\nsetEQUIV = \"\"\nincFILE = \"\"\n")
      fprintf(port "auCdlDefNetlistProc = \"ansCdlSubcktCall\"\nallowNetNamesBeginningWithDigits = 'nil\n")
    )
    close(port)
    port = outfile(strcat(work "/orders.sx"))
    fprintf(port "%L\n" rows)
    close(port)
    foreach(key annotations annotationRows = cons(annotations[key] annotationRows))
    port = outfile(strcat(work "/annotations.sx"))
    fprintf(port "%L\n" annotationRows)
    close(port)
    foreach(key symbolSelections symbolRows = cons(symbolSelections[key] symbolRows))
    port = outfile(strcat(work "/symbols.sx"))
    fprintf(port "%L\n" symbolRows)
    close(port)
    when(selectionBoxes foreach(key selectionBoxes selectionRows = cons(selectionBoxes[key] selectionRows)))
    port = outfile(strcat(work "/selection-boxes.sx"))
    fprintf(port "%L\n" selectionRows)
    close(port)
    port = outfile(strcat(work "/request.sx"))
    fprintf(port "%L\n" list("rnl-export-job-v7" rnlInstallDirectory lib cell view cds
      getWorkingDir() destination hierarchy split scale car(getInstallPath()) format hasSymbol scope capture schematicViews symbolViews if(symbolView then symbolView else "") exportSelectionBox exportSymbols))
    close(port)
    command = strcat(rnlShellQuote(strcat(rnlInstallDirectory "/bin/rnl-virtuoso")) " "
      rnlShellQuote(strcat(work "/request.sx")))
    rnlActiveJob = list(nil 'directory work 'destination destination 'notify notify
      'entry strcat(cell if(format == "spectre" then ".scs" else ".sp")))
    rnlLastJob = rnlActiveJob
    rnlActiveJob->process = ipcBeginProcess(command "" 'rnlProcessOutput 'rnlProcessOutput 'rnlProcessDone)
    printf("RNL: exporting %s/%s/%s; logs: %s\n" lib cell view work)
    work
  )
)

procedure(rnlProcessOutput(process data)
  printf("RNL: %s" data)
)

procedure(rnlProcessDone(process status)
  let((job message port line)
    job = rnlActiveJob
    rnlActiveJob = nil
    when(job
      job->status = status
      if(boundp('rnlActiveBatch) && rnlActiveBatch then
        rnlBatchProcessDone(job status)
      else
      if(status == 0 then
        message = strcat("Export complete: " job->destination "/" job->entry "\nLogs: " job->directory)
      else
        message = "Export failed."
        port = infile(strcat(job->directory "/error.txt"))
        when(port
          when(gets(line port) message = strcat(message "\n" line))
          close(port))
        message = strcat(message "\nLogs: " job->directory)
      )
      if(job->notify then rnlNotice(message) else printf("RNL: %s\n" message))
      )
    )
  )
)

procedure(rnlExportCallback(form)
  let((result)
    result = errset(rnlStartExport(car(form->rnlSource) cadr(form->rnlSource) nil
      form->outputDirectory->value form->includeHierarchy->value
      form->fileMode->value == "Separate files" form->drawingScale->value
      if(form->netlistFormat->value == "Spectre (simulation)" then "spectre" else "cdl")
      if(!form->includeExternal || form->includeExternal->value then "" else car(form->rnlSource)) nil t form->schematicViews->value form->symbolViews->value form->exportSelectionBox->value form->autoCheckSave->value form->exportSymbols->value) t)
    hiSetCallbackStatus(form if(result then t else nil))
    unless(result rnlNotice("Export could not start. See the CIW error message."))
  )
)

procedure(rnlShowExport()
  let((cv)
    cv = geGetEditCellView()
    unless(cv && cv->cellViewType == "schematic"
      error("Open a schematic editor before choosing Export RNL.\n"))
    unless(rnlExportForm && rnlExportForm->exportSymbols
      rnlExportForm = hiCreateAppForm(?name 'rnlExportFormV094 ?formTitle "Export RNL"
        ?callback 'rnlExportCallback ?buttonLayout 'OKCancel ?unmapAfterCB t ?dontBlock t
        ?fields list(
          hiCreateStringField(?name 'sourceDesign ?prompt "Source" ?editable nil)
          hiCreateStringField(?name 'schematicViews ?prompt "Schematic views (highest priority first)" ?value "schematic")
          hiCreateStringField(?name 'symbolViews ?prompt "Symbol views (highest priority first)" ?value "symbol")
          hiCreateBooleanButton(?name 'autoCheckSave ?buttonText "Automatically Check & Save stale schematics" ?value t)
          hiCreateBooleanButton(?name 'exportSelectionBox ?buttonText "Export selection box" ?value t)
            hiCreateBooleanButton(?name 'exportSymbols ?buttonText "Export symbols.rnl separately" ?value nil)
          hiCreateStringField(?name 'outputDirectory ?prompt "New output directory")
          hiCreateCyclicField(?name 'netlistFormat ?prompt "Netlist format"
            ?choices list("Spectre (simulation)" "SPICE / CDL (LVS)") ?value "Spectre (simulation)")
          hiCreateBooleanButton(?name 'includeHierarchy ?buttonText "Include subcircuits" ?value t)
          hiCreateBooleanButton(?name 'includeExternal ?buttonText "Include referenced external cells" ?value t)
          hiCreateCyclicField(?name 'fileMode ?prompt "Subcircuits"
            ?choices list("Combined file" "Separate files") ?value "Separate files")
          hiCreateFloatField(?name 'drawingScale ?prompt "EDIF units per RNL unit" ?value 10.0)
        )))
    ; Upgrade a form kept alive by a previously loaded version of this plugin.
    unless(member("Spectre (simulation)" rnlExportForm->netlistFormat->choices)
      rnlExportForm->netlistFormat->choices = list("Spectre (simulation)" "SPICE / CDL (LVS)")
      rnlExportForm->netlistFormat->value = "Spectre (simulation)")
    rnlExportForm->rnlSource = list(cv->libName cv->cellName cv->viewName)
    rnlExportForm->sourceDesign->value = sprintf(nil "%s / %s / %s" cv->libName cv->cellName cv->viewName)
    rnlExportForm->outputDirectory->value = strcat(getWorkingDir() "/" cv->cellName "_rnl")
    hiDisplayForm(rnlExportForm)
  )
)

procedure(rnlMenuTrigger(args)
  list(rnlMenu)
)

procedure(rnlInstall(directory)
  let((info cv)
    unless(isFile(strcat(directory "/bin/rnl-virtuoso"))
      error("RNL Go backend not found under %s. Extract the complete installation package.\n" directory))
    rnlInstallDirectory = directory
    unless(rnlMenu
      rnlMenu = hiCreatePulldownMenu('rnlMenu "RNL" list(
        hiCreateMenuItem(?name 'rnlExportItem ?itemText "Export RNL..." ?callback "rnlShowExport()"))))
    info = deGetAppInfo("schematic")
    ; IC6.1.8 supports multiple triggers. Add ours without removing site hooks.
    unless(member('rnlMenuTrigger info->userMenuTrigList)
      deRegUserTriggers("schematic" nil 'rnlMenuTrigger nil))
    foreach(window hiGetWindowList()
      cv = geGetWindowCellView(window)
      when(cv && cv->cellViewType == "schematic" && !member('rnlMenu hiGetBannerMenus(window))
        hiInsertBannerMenu(window rnlMenu length(hiGetBannerMenus(window)))))
    rnlInstallCIW()
    printf("RNL: installed. Choose RNL > Export RNL... in a schematic editor.\n")
    t
  )
)

; piport identifies the file currently being loaded, including nested loads.
; Resolve it once at load time; later menu callbacks run with a different port.
let((file directory)
  file = simplifyFilename(get_filename(piport))
  directory = substring(file 1 strlen(file)-strlen(car(reverse(parseString(file "/")))))
  load(strcat(directory "/rnl-library.il"))
  load(strcat(directory "/rnl-api.il"))
  rnlInstall(directory)
)
