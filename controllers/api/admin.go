// Copyright 2017 ETH Zurich
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/netsec-ethz/scion-coord/config"
	"github.com/netsec-ethz/scion-coord/controllers"
	"github.com/netsec-ethz/scion-coord/controllers/middleware"
	"github.com/netsec-ethz/scion-coord/email"
	"github.com/netsec-ethz/scion-coord/models"
)

type AdminController struct {
	controllers.HTTPController
}

type accountData struct {
	AccountID     string
	AccountSecret string
}

type adminPageData struct {
	User         user
	EmailMessage string
	Account      accountData
}

type invitationInfo struct {
	FirstName    string
	LastName     string
	Email        string
	Organisation string
}

type invitationsData []invitationInfo

var invitationsTemplate = "invitation.html"

func (c AdminController) AdminInformation(w http.ResponseWriter, r *http.Request) {
	user, account, err := populateUserData(r)
	if err != nil {
		log.Printf("Error authenticating user: %v", err)
		c.Forbidden(w, err, "Error authenticating user")
		return
	}

	// TODO (mlegner): Fill in template except FirstName and LastName
	text, err := ioutil.ReadFile(email.MailTemplatePath(invitationsTemplate))

	adminData := adminPageData{
		User:         user,
		EmailMessage: string(text),
		Account:      account,
	}

	c.JSON(&adminData, w, r)
	return
}

func preregisterAndSendInvitation(userSession *models.Session, invitation *invitationInfo) error {
	// register the user without password
	account := invitation.Email // use the user's email as a unique account
	user, err := models.RegisterUser(account, invitation.Organisation,
		invitation.Email,
		"", invitation.FirstName, invitation.LastName)
	if err != nil {
		return err
	}

	err = user.UpdateVerified(true)
	if err != nil {
		return err
	}

	data := struct {
		FirstName        string
		LastName         string
		InviterFirstName string
		InviterLastName  string
		HostAddress      string
		UUID             string
	}{
		FirstName:        invitation.FirstName,
		LastName:         invitation.LastName,
		InviterFirstName: userSession.First,
		InviterLastName:  userSession.Last,
		HostAddress:      config.HTTPHostAddress,
		UUID:             user.VerificationUUID,
	}

	email.ConstructFromTemplateAndSend(
		"invitation.html",
		"[SCIONLab] Invitation to join the SCION network",
		data,
		"scion-invitation",
		invitation.Email,
		false)

	return nil
}

func (c AdminController) SendInvitationEmails(w http.ResponseWriter, r *http.Request) {

	// parse the JSON coming from the client
	decoder := json.NewDecoder(r.Body)
	var invitations invitationsData

	// check if the parsing succeeded
	if err := decoder.Decode(&invitations); err != nil {
		log.Printf("Error decoding json data for email invitations: %v", err)
		c.Error500(w, err, "Error decoding json data for email invitations")
		return
	}

	session, userSession, err := middleware.GetUserSession(r)
	if session == nil || err != nil {
		log.Printf("No user session found: %v", err)
		c.Forbidden(w, err, "No user session found")
	}

	var errorEmails []string
	var errors []string
	for _, invitation := range invitations {
		err := preregisterAndSendInvitation(userSession, &invitation)
		if err != nil {
			log.Printf("Error sending invitation email to %v: %v", invitation.Email, err)
			errorEmails = append(errorEmails, invitation.Email)
			errors = append(errors, controllers.Verbosity(err, "Could not send email to user %v", invitation.Email))
		} else {
			errors = append(errors, "")
		}
	}

	if len(errors) == 0 {
		c.Plain("", w, r)
		return
	} else {
		c.JSON(map[string][]string{"messages": errors, "emails": errorEmails}, w, r)
		return
	}
}
