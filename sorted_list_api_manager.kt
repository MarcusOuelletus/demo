/*
This class is used in a infinite list from a personal project of mine, loadAll, loadBefore, and loadAfter can be called to make a server call via IInfiniteListData.
The returned rows will be added to our infinite list in the correct order and will scroll the list accordingly given the scroll related function parameters.
This class will also make sure that if a public function such as loadAll is called repeatedly while a request is in flight, those requests are ignored until the response arrives.
*/

class InfiniteListDataManager(
    private val coroutineScope: CoroutineScope,
    private val data: IInfiniteListData,
    private val indexZeroIsNewest: Boolean = false,
    private val scrollToZero: () -> Unit,
    private val scrollToInfinity: () -> Unit,
    private val scrollToIndex: (index: Int, onlyIfAboveMinVisible: Boolean) -> Unit,
    private val shouldScrollOnNewRow: () -> Boolean,
    private val identifier: String,
) {
    private val loadAllChannel = Channel<InfiniteList.LoadMoreParams>(1, onBufferOverflow = BufferOverflow.DROP_LATEST)
    private val loadAfterChannel = Channel<InfiniteList.LoadMoreParams>(1, onBufferOverflow = BufferOverflow.DROP_LATEST)
    private val loadBeforeChannel = Channel<InfiniteList.LoadMoreParams>(1, onBufferOverflow = BufferOverflow.DROP_LATEST)

    // coroutineScope is acquired using rememberCoroutineScope from the InfiniteList Composable
    // these for loops and channels will automatically be cancelled and closed when the composable
    // is destroyed. This function ensures that only one load after is ever called at a time.
    // Infinite list handler calls loadAfter on the main thread and it adds to loadAfterChannel
    // if it is empty, if it isn't empty then a load after is already queued up for when it finishes
    // the one it is currently performing.
    fun start() {
        coroutineScope.launch { for (params in loadAllChannel) performLoadAll() }
        coroutineScope.launch { for (params in loadAfterChannel) performLoadAfter(params) }
        coroutineScope.launch { for (params in loadBeforeChannel) performLoadBefore(params) }
        coroutineScope.launch { for (newRow in data.newRowsChannel) handleNewRows(mutableListOf(newRow), InfiniteList.LoadMoreParams(scroll = shouldScrollOnNewRow())) }
    }

    suspend fun loadAll() = loadAllChannel.send(InfiniteList.LoadMoreParams())
    suspend fun loadAfter(params: InfiniteList.LoadMoreParams) = loadAfterChannel.send(params)
    suspend fun loadBefore(params: InfiniteList.LoadMoreParams) = loadBeforeChannel.send(params)

    private fun scrollIfShould(params: InfiniteList.LoadMoreParams) {
        val shouldScroll = if (params.scroll) shouldScrollOnNewRow() else false

        if (shouldScroll) {
            if (indexZeroIsNewest) scrollToZero() else scrollToInfinity()
        }
    }

    private suspend fun performLoadAll() {
        data.loading.postValue(ListLoadingState.ALL)
        handleNewRows(withContext(Dispatchers.IO) { data.loadAll().toMutableList() }, InfiniteList.LoadMoreParams(scroll = true))
        data.loading.postValue(ListLoadingState.NONE)
    }

    private suspend fun performLoadAfter(params: InfiniteList.LoadMoreParams) {
            val timestamp = withContext(Dispatchers.Main) {
                if (indexZeroIsNewest) data.rows.firstOrNull()?.getTimestamp()
                else data.rows.lastOrNull()?.getTimestamp()
            } ?: 0

            data.loading.postValue(ListLoadingState.NEWER)
            handleNewRows(withContext(Dispatchers.IO) { data.loadAfter(timestamp).toMutableList() }, params)
            data.loading.postValue(ListLoadingState.NONE)
    }

    private suspend fun performLoadBefore(params: InfiniteList.LoadMoreParams) {
            val timestamp = withContext(Dispatchers.Main) {
                if (indexZeroIsNewest) data.rows.lastOrNull()?.getTimestamp()
                else data.rows.firstOrNull()?.getTimestamp()
            } ?: Long.MAX_VALUE

            data.loading.postValue(ListLoadingState.OLDER)
            handleNewRows(withContext(Dispatchers.IO) { data.loadBefore(timestamp).toMutableList() }, params)
            data.loading.postValue(ListLoadingState.NONE)
        }

    private suspend fun handleNewRows(r: MutableList<IInfiniteListRow>, params: InfiniteList.LoadMoreParams) {
        if (r.isEmpty()) return

        withContext(Dispatchers.Main) {
            // remove values if they are already in the list
            val i = r.listIterator()
            while (i.hasNext()) {
                val v = i.next()
                data.rows.find { it.getKey() == v.getKey() }?.also {
                    i.remove()
                }
            }

            if (r.isEmpty()) return@withContext

            if (!indexZeroIsNewest) {
                if (r.size == 1 || r[0].getTimestamp() < r[1].getTimestamp()) {
                    handleASCListNewRows(r, params)
                } else {
                    handleASCListNewRows(r.reversed(), params)
                }
            } else {
                if (r.size == 1 || r[0].getTimestamp() > r[1].getTimestamp()) {
                    handleDESCListNewRows(r, params)
                } else {
                    handleDESCListNewRows(r.reversed(), params)
                }
            }
        }
    }

    // handleDESCListNewRows - the list parameter needs to be in DESC order
    private fun handleDESCListNewRows(r: List<IInfiniteListRow>, params: InfiniteList.LoadMoreParams) {
        if (data.rows.isEmpty()) {
            data.rows.addAll(r)
            scrollIfShould(params)
            return
        }

        val newRows = r.toMutableList()

        while (newRows.isNotEmpty()) {
            // get the index at which to add the first value in our newRows lists (list is DESC)
            // it will be the index of the first item with a larger timestamp
            val indexToInsert = data.rows.binarySearch {
                (newRows[0].getTimestamp() - it.getTimestamp()).toInt()
            }.let { if (it < 0) (it * -1) - 1 else it }

            val nextOlderTimestamp = if (indexToInsert >= data.rows.size) 0 else data.rows[indexToInsert].getTimestamp()

            // how many from our newRows list from index 0 go before nextNewerTimestamp
            // if we have rows in newRows with a timestamp larger than nextNewerTimestamp
            // insert them in our next iteration
            var quantityToAdd = 0
            newRows.forEachIndexed { _, row ->
                if (row.getTimestamp() > nextOlderTimestamp) {
                    quantityToAdd++
                } else {
                    // if one of our newRows values is newer than nextNewerTimestamp, stop counting
                    return@forEachIndexed
                }
            }

            if (quantityToAdd == 0) break

            data.rows.addAll(indexToInsert, newRows.take(quantityToAdd))
            scrollIfShould(params)

            // remove the items in newRows that have been added and loop again if we haven't added all
            newRows.removeIf {it.getTimestamp() > nextOlderTimestamp}
        }
    }

    // handleASCListNewRows - the list parameter needs to be in ASC order
    private fun handleASCListNewRows(r: List<IInfiniteListRow>, params: InfiniteList.LoadMoreParams) {
        if (data.rows.isEmpty()) {
            data.rows.addAll(r)
            scrollIfShould(params)
            return
        }

        val newRows = r.toMutableList()

        while (newRows.isNotEmpty()) {
            // get the index at which to add the first value in our newRows lists (list is ASC)
            // it will be the index of the first item with a larger timestamp
            val indexToInsert = data.rows.binarySearch {
                (it.getTimestamp() - newRows[0].getTimestamp()).toInt()
            }.let { if (it < 0) (it * -1) - 1 else it }

            val nextNewerTimestamp = if (indexToInsert >= data.rows.size) Long.MAX_VALUE else data.rows[indexToInsert].getTimestamp()

            // how many from our newRows list from index 0 go before nextNewerTimestamp
            // if we have rows in newRows with a timestamp larger than nextNewerTimestamp
            // insert them in our next iteration
            var quantityToAdd = 0
            newRows.forEachIndexed { _, row ->
                if (row.getTimestamp() < nextNewerTimestamp) {
                    quantityToAdd++
                } else {
                    // if one of our newRows values is newer than nextNewerTimestamp, stop counting
                    return@forEachIndexed
                }
            }

            if (quantityToAdd == 0) break

            data.rows.addAll(indexToInsert, newRows.take(quantityToAdd))
            scrollIfShould(params)

            // remove the items in newRows that have been added and loop again if we haven't added all
            newRows.removeIf {it.getTimestamp() < nextNewerTimestamp}
        }

        if (newRows.isNotEmpty()) {
            data.rows.addAll(newRows)
            scrollIfShould(params)
        }
    }
}
