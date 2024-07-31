/*===========================================================================*\

\*===========================================================================*/

package pobo

import "fmt"

var (
	Name        string
	Version     string
	Copyright   string
	License     string
	AuthorName  string
	AuthorEmail string
)

func Banner() string {
	var banner string
	banner += fmt.Sprintf("\n")
	banner += fmt.Sprintf(" ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~\n")
	banner += fmt.Sprintf(" ~                                        ~\n")
	banner += fmt.Sprintf(" ~   ██████╗  ██████╗ ██████╗  ██████╗    ~\n")
	banner += fmt.Sprintf(" ~   ██╔══██╗██╔═══██╗██╔══██╗██╔═══██╗   ~\n")
	banner += fmt.Sprintf(" ~   ██████╔╝██║   ██║██████╔╝██║   ██║   ~\n")
	banner += fmt.Sprintf(" ~   ██╔═══╝ ██║   ██║██╔══██╗██║   ██║   ~\n")
	banner += fmt.Sprintf(" ~   ██║     ╚██████╔╝██████╔╝╚██████╔╝   ~\n")
	banner += fmt.Sprintf(" ~   ╚═╝      ╚═════╝ ╚═════╝  ╚═════╝    ~\n")
	banner += fmt.Sprintf(" ~                                        ~\n")
	banner += fmt.Sprintf(" ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~\n")
	banner += fmt.Sprintf(" Created by: %s <%s>\n", AuthorName, AuthorEmail)
	return banner
}
