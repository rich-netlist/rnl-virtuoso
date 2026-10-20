; RNL schematic export. Loading this file installs the menu automatically.
; The native exporters read saved views; no source cellview is modified here.
unless(boundp('rnlActiveJob) rnlActiveJob = nil)
unless(boundp('rnlMenu) rnlMenu = nil)
unless(boundp('rnlExportForm) rnlExportForm = nil)

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

procedure(rnlCheckSaved(cv)
  when(cv->modifiedButNotSaved || !dbIsConnCurrent(cv)
    error("Check and Save %s/%s/%s before exporting.\n"
      cv->libName cv->cellName cv->viewName))
)

procedure(rnlCollectOrders(cv seen orders @optional (simulator "spectre") (scope ""))
  let((key master cdf sim child result)
    key = list(cv->libName cv->cellName cv->viewName)
    unless(seen[key]
      seen[key] = t
      rnlCheckSaved(cv)
      ; The selected cell's interface is needed when wrapping it as a subckt.
      cdf = cdfGetBaseCellCDF(ddGetObj(cv->libName cv->cellName))
      sim = when(cdf get(cdf->simInfo concat(simulator)))
      orders[list(cv->libName cv->cellName)] = list(cv->libName cv->cellName
        when(sim foreach(mapcar terminal sim->termOrder
          if(stringp(terminal) then terminal else get_pname(terminal)))))
      foreach(inst cv->instances
        master = inst->master
        when(master->modifiedButNotSaved
          error("Save symbol %s/%s/%s before exporting.\n" master->libName master->cellName master->viewName))
        key = list(master->libName master->cellName)
        unless(orders[key]
          cdf = cdfGetBaseCellCDF(ddGetObj(master->libName master->cellName))
          sim = when(cdf get(cdf->simInfo concat(simulator)))
          orders[key] = list(master->libName master->cellName
            when(sim foreach(mapcar terminal sim->termOrder
              if(stringp(terminal) then terminal else get_pname(terminal)))))
        )
        ; Native netlisting also stops at nlAction, including markers such as
        ; analogLib/gnd and basic/noConn. Their schematic is not a child circuit.
        unless(member(master->nlAction list("ignore" "stop")) ||
          ddGetObj(master->libName master->cellName simulator)
          when((scope == "" || master->libName == scope) && ddGetObj(master->libName master->cellName "schematic")
            child = dbOpenCellViewByType(master->libName master->cellName "schematic" nil "r")
            when(child
              unwindProtect(
                unless(member(child->nlAction list("ignore" "stop"))
                  rnlCollectOrders(child seen orders simulator scope))
                dbClose(child)))))
      )
    )
    foreach(k orders result = cons(orders[k] result))
    result
  )
)

procedure(rnlStartExport(lib cell view destination hierarchy split scale @optional (format "spectre") (scope "") (capture nil))
  let((cv rows work port cds command simulator hasSymbol symbol)
    when(rnlActiveJob error("An RNL export is already running.\n"))
    when(boundp('rnlActiveBatch) && rnlActiveBatch && !rnlActiveBatch->starting
      error("An RNL library export is already running.\n"))
    unless(destination && destination != "" error("Choose a new output directory.\n"))
    unless(scale && scale > 0 error("Drawing scale must be positive.\n"))
    unless(member(format list("spectre" "cdl")) error("Choose spectre or cdl.\n"))
    simulator = if(format == "spectre" then "spectre" else "auCdl")
    cv = dbOpenCellViewByType(lib cell view nil "r")
    unless(cv error("Cannot open %s/%s/%s.\n" lib cell view))
    unwindProtect(
      rows = rnlCollectOrders(cv makeTable('rnlSeen nil) makeTable('rnlOrders nil) simulator scope)
      dbClose(cv))
    hasSymbol = ddGetObj(lib cell "symbol") != nil
    when(hasSymbol
      symbol = dbOpenCellViewByType(lib cell "symbol" nil "r")
      unless(symbol error("Cannot open the current cell's symbol view.\n"))
      unwindProtect(
        when(symbol->modifiedButNotSaved error("Save %s/%s/symbol before exporting.\n" lib cell))
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
      fprintf(port "simViewList = '(\"spectre\" \"schematic\" \"veriloga\" \"veriloga_ans\")\nsimStopList = '(\"spectre\" \"veriloga\" \"veriloga_ans\")\n")
    else
      ; Same preservation options as File > Export > CDL, Analog mode.
      fprintf(port "simViewList = '(\"auCdl\" \"schematic\")\nsimStopList = '(\"auCdl\")\n")
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
    port = outfile(strcat(work "/request.sx"))
    fprintf(port "%L\n" list("rnl-export-job-v4" rnlInstallDirectory lib cell view cds
      getWorkingDir() destination hierarchy split scale car(getInstallPath()) format hasSymbol scope capture))
    close(port)
    command = strcat(rnlShellQuote(strcat(rnlInstallDirectory "/bin/rnl-virtuoso")) " "
      rnlShellQuote(strcat(work "/request.sx")))
    rnlActiveJob = list(nil 'directory work 'destination destination
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
      rnlNotice(message)
      )
    )
  )
)

procedure(rnlExportCallback(form)
  let((result)
    result = errset(rnlStartExport(car(form->rnlSource) cadr(form->rnlSource) caddr(form->rnlSource)
      form->outputDirectory->value form->includeHierarchy->value
      form->fileMode->value == "Separate files" form->drawingScale->value
      if(form->netlistFormat->value == "Spectre (simulation)" then "spectre" else "cdl")) t)
    hiSetCallbackStatus(form if(result then t else nil))
    unless(result rnlNotice("Export could not start. See the CIW error message."))
  )
)

procedure(rnlShowExport()
  let((cv)
    cv = geGetEditCellView()
    unless(cv && cv->cellViewType == "schematic"
      error("Open a schematic editor before choosing Export RNL.\n"))
    unless(rnlExportForm
      rnlExportForm = hiCreateAppForm(?name 'rnlExportForm ?formTitle "Export RNL"
        ?callback 'rnlExportCallback ?buttonLayout 'OKCancel ?unmapAfterCB t ?dontBlock t
        ?fields list(
          hiCreateStringField(?name 'sourceDesign ?prompt "Source" ?editable nil)
          hiCreateStringField(?name 'outputDirectory ?prompt "New output directory")
          hiCreateCyclicField(?name 'netlistFormat ?prompt "Netlist format"
            ?choices list("Spectre (simulation)" "SPICE / CDL (LVS)") ?value "Spectre (simulation)")
          hiCreateBooleanButton(?name 'includeHierarchy ?buttonText "Include subcircuits" ?value t)
          hiCreateCyclicField(?name 'fileMode ?prompt "Subcircuits"
            ?choices list("Combined file" "Separate files") ?value "Combined file")
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
  rnlInstall(directory)
)
