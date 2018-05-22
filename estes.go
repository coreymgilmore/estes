/*Package estes provides tooling to connect to the Estes Freight API.  This is for truck shipments,
not small parcels.  Think LTL (less than truckload) shipments.  This code was created off the Estes API
documentation.  This uses and XML SOAP API.

You will need to have a Estes account and register for access to use this.

Currently this package can perform:
- pickup requests

To create a pickup request:
- Set test or production mode (SetProductionMode()).
- Set shipper information.
- Set shipment data.
- Request the pickup.
- Check for any errors.
*/
package estes

import (
	"bytes"
	"encoding/xml"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

//api urls
const (
	estesTestURL       = "https://apitest.estes-express.com/tools/pickup/request/v1.0"
	estesProductionURL = "https://api.estes-express.com/tools/pickup/request/v1.0"
)

//estesURL is set to the test URL by default
//This is changed to the production URL when the SetProductionMode function is called
//Forcing the developer to call the SetProductionMode function ensures the production URL is only used
//when actually needed.
var estesURL = estesTestURL

//timeout is the default time we should wait for a reply from Ward
//You may need to adjust this based on how slow connecting to Ward is for you.
//10 seconds is overly long, but sometimes Ward is very slow.
var timeout = time.Duration(10 * time.Second)

//base XML data
const (
	soapenvAttr = "http://schemas.xmlsoap.org/soap/envelope/"
	estAttr     = "http://estespickup.base.ws.provider.soapws.pickupRequest"
)

//PickupRequest is the main body of the xml request
type PickupRequest struct {
	XMLName xml.Name `xml:"soapenv:Envelope"`

	SoapenvAttr string `xml:"xmlns:soapenv,attr"`
	EstAttr     string `xml:"xmlns:est,attr"`

	PickupRequestInput PickupRequestInput `xml:"soapenv:Body>est:createPickupRequestWS>pickupRequestInput"`
}

//PickupRequestInput is the data on the pickup request
type PickupRequestInput struct {
	//required
	Shipper            Shipper `xml:"shipper"`
	PickupDate         string  `xml:"pickupDate"`      //yyyy-mm-dd
	PickupStartTime    string  `xml:"pickupStartTime"` //hhmm
	PickupEndTime      string  `xml:"pickupEndTime"`   //hhmm
	TotalPieces        uint    `xml:"totalPieces"`     //pieces
	TotalWeight        float64 `xml:"totalWeight"`
	TotalHandlingUnits uint    `xml:"totalHandlingUnits"` //skids

	//optional
	RequestNumber string `xml:"requestNumber"`
	WhoRequested  string `xml:"whoRequested"`
}

//Shipper is data on where a shipment is coming from
type Shipper struct {
	//required
	ShipperName string `xml:"shipperName"`

	//optional
	ShipperAddress Address `xml:"shipperAddress>addressInfo"`
	ShipperContact Contact `xml:"shipperContacts>shipperContact"`
}

//Address holds an address
type Address struct {
	//required
	AddressLine1  string `xml:"addressLine1"`
	City          string `xml:"city"`
	StateProvince string `xml:"stateProvince"`
	PostalCode    string `xml:"postalCode"`
	Country       string `xml:"countryAbbrev"`

	//optional
	AddressLine2 string `xml:"addressLine2"`
}

//Contact holds a contact
type Contact struct {
	Name  Name   `xml:"name"`
	Email string `xml:"email"`
	Phone Phone  `xml:"phone"`
}

//Name holds a contact's name
type Name struct {
	First  string `xml:"firstName"`
	Middle string `xml:"middleName"`
	Last   string `xml:"lastName"`
}

//Phone holds a contact's phone number
type Phone struct {
	AreaCode string `xml:"areaCode"` //first 3
	Number   string `xml:"number"`   //last 7, only numbers
}

//SuccessfulPickupRequest is the format of a successful pickup request
type SuccessfulPickupRequest struct {
	XMLName  xml.Name                    `xml:"Envelope"` //don't need "soapenv"
	Response CreatePickupRequestResponse `xml:"Body>createPickupRequestWSResponse"`
}

//CreatePickupRequestResponse is the pickup request confirmation data
type CreatePickupRequestResponse struct {
	RequestNumber string `xml:"requestNumber"`
}

//ErrorPickupRequest is the format of an error returned when scheduling a pickup
type ErrorPickupRequest struct {
	XMLName     xml.Name `xml:"error"`
	Code        string   `xml:"code"`
	Description string   `xml:"description"`
	BadData     string   `xml:"badData"`
}

//SetProductionMode chooses the production url for use
func SetProductionMode(yes bool) {
	if yes {
		estesURL = estesProductionURL
	}
	return
}

//SetTimeout updates the timeout value to something the user sets
//use this to increase the timeout if connecting to Ward is really slow
func SetTimeout(seconds time.Duration) {
	timeout = time.Duration(seconds * time.Second)
	return
}

//RequestPickup performs the call to the estes api to schedule a pickup
func (p *PickupRequestInput) RequestPickup(estesUsername, estesPassword string) (responseData SuccessfulPickupRequest, err error) {
	//build the complete pickup request object
	pickup := PickupRequest{
		SoapenvAttr:        soapenvAttr,
		EstAttr:            estAttr,
		PickupRequestInput: *p,
	}

	//convert the pickup request to an xml
	xmlBytes, err := xml.Marshal(pickup)
	if err != nil {
		err = errors.Wrap(err, "estes.RequestPickup - could not marshal xml")
		return
	}

	//make the call to the estes API
	//set a timeout since golang doesn't set one by default and we don't want this to hang forever
	httpClient := http.Client{
		Timeout: timeout,
	}

	log.Println(string(xmlBytes))

	req, err := http.NewRequest("POST", estesProductionURL, bytes.NewReader(xmlBytes))
	if err != nil {
		err = errors.Wrap(err, "estes.RequestPickup - could not make build request")
		return
	}

	req.SetBasicAuth(estesUsername, estesPassword)
	req.Header.Add("Content-Type", "text/xml")

	res, err := httpClient.Do(req)
	if err != nil {
		err = errors.Wrap(err, "estes.RequestPickup - could not make post request")
		return
	}

	//read the response
	body, err := ioutil.ReadAll(res.Body)
	defer res.Body.Close()
	if err != nil {
		err = errors.Wrap(err, "estes.RequestPickup - could not read response 1")
		return
	}

	err = xml.Unmarshal(body, &responseData)
	if err != nil {
		log.Println(string(body))
		err = errors.Wrap(err, "estes.RequestPickup - could not read response 2")
		return
	}

	//check if data was returned meaning request was successful
	//if not, reread the response data and log it
	if responseData.Response.RequestNumber == "" {
		log.Println("estes.RequestPickup - pickup request failed")
		log.Printf(string(body))

		var errorData ErrorPickupRequest
		xml.Unmarshal(body, &errorData)

		err = errors.New("estes.RequestPickup - pickup request failed")
		log.Println(errorData)
		return
	}

	//pickup request successful
	//response data will have confirmation info
	return

}
