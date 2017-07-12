/*
Package domainsockets implements an OS level procedure calling service based
on the Unix Domain Sockets IPC.

This implementation is simplistic in that it does not try to duplicate more
complex communication mechansims, such as message splitting (into packets),
assembly of disjointed messages, etc...

Instead a client sends an entire message across the socket and receives
a response.  Calls are syncronous.  The client can send multiple messages
simultaneously (calls are thread safe).

At this point, local security is a concern, as there is not authorization
control against users on the same OS.  This may be a concern depending on if
the OS is shared between untrusted users or if the tmp directory is exposed
to other containers instead of being container local only.

Finally, compatibility is only currently guarenteed between client and servers
using the same version of code.  Some of the underlying mechansims may be
changed in the future. Once this is completely settled, this warning will
go away.
*/
package domainsockets
