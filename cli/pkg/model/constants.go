package model

type PhotosStore string

const (
	KVConfig           PhotosStore = "kvConfig"
	RemoteAlbums       PhotosStore = "remoteAlbums"
	RemoteFiles        PhotosStore = "remoteFiles"
	RemoteAlbumEntries PhotosStore = "remoteAlbumEntries"
	UploadStates       PhotosStore = "uploadStates"
	FileHashes         PhotosStore = "fileHashes"
	WatchStates        PhotosStore = "watchStates"
	WatchFiles         PhotosStore = "watchFiles"
)

const (
	CollectionsSyncKey        = "lastCollectionSync"
	CollectionsFileSyncKeyFmt = "collectionFilesSync-%d"
	AuthenticatorSyncKey      = "lastAuthenticatorSync"
)

type ContextKey string

const (
	FilterKey ContextKey = "export_filter"
)
