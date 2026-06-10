package lusciimodels

// Generated from OpenAPI specification
// Note: For date-time fields, consider using time.Time from "time" package

// AlertsReportResponse - Fields of patient alert response.
type AlertsReportResponse struct {
	// The unique identifier of the patient.
	PatientUUID *UuidSchema `json:"Patient_UUID,omitempty"`
	// The group the patient belongs to.
	Group *string `json:"Group,omitempty"`
	// The program the patient is enrolled in.
	Program *string `json:"Program,omitempty"`
	// The protocol the patient is following.
	Protocol *string `json:"Protocol,omitempty"`
	// The flag indicating an alert.
	AlertFlag *string `json:"Alert_Flag,omitempty"`
	// The type of alert.
	AlertType *string `json:"Alert_Type,omitempty"`
	// The name of the measurement.
	MeasurementName *string `json:"Measurement_Name,omitempty"`
	// The date and time when the alert was created.
	AlertCreatedDate *DateTimeSchema `json:"Alert_Created_Date,omitempty"`
	// The type of alert processing.
	AlertProcessedType interface{} `json:"Alert_Processed_Type,omitempty"`
	// The action taken for alert processing.
	AlertProcessedAction interface{} `json:"Alert_Processed_Action,omitempty"`
	// The date and time when the alert was processed.
	AlertProcessedAt *DateTimeSchema `json:"Alert_Processed_At,omitempty"`
	// The unique identifier of the user who processed the alert.
	AlertProcessedByUUID *UuidSchema `json:"Alert_Processed_By_UUID,omitempty"`
	// The role of the user who processed the alert.
	AlertProcessedByRole interface{} `json:"Alert_Processed_By_Role,omitempty"`
	// Additional information about the alert processing.
	AlertProcessedAdditionalInfo interface{} `json:"Alert_Processed_Additional_Info,omitempty"`
	// The unique identifier of the alert.
	Id *UuidSchema `json:"Id,omitempty"`
}

// AlertTransformer - Updated Alert details
type AlertTransformer struct {
	// Alert's identifier
	Id *UuidSchema `json:"id,omitempty"`
	// Alert group's identifier
	GroupId *UuidSchema `json:"groupId,omitempty"`
	// Protocol identifier, if any
	ProtocolId interface{} `json:"protocolId,omitempty"`
	// Patient identifier
	UserId *UuidSchema `json:"userId,omitempty"`
	// Measurement identifier, if any
	MeasurementId interface{} `json:"measurementId,omitempty"`
	// Program identifier, if any
	ProgramId interface{} `json:"programId,omitempty"`
	// Instrument identifier, if any
	InstrumentId interface{} `json:"instrumentId,omitempty"`
	// Flag value that triggered the alert
	Flag interface{} `json:"flag,omitempty"`
	// The date and time when the alert was processed.
	ProcessedAt interface{} `json:"processedAt,omitempty"`
	// User who processed the alert
	ProcessedBy interface{} `json:"processedBy,omitempty"`
	// The date and time when the alert was escalated.
	EscalatedAt interface{} `json:"escalatedAt,omitempty"`
	// User who escalated the alert
	EscalatedBy interface{} `json:"escalatedBy,omitempty"`
	// The date and time when the alert was created.
	CreatedAt *IsoDateTimeSchema `json:"createdAt,omitempty"`
	// The date and time when the alert was last updated.
	UpdatedAt *IsoDateTimeSchema `json:"updatedAt,omitempty"`
	// Alert type identifier
	Type *string `json:"type,omitempty"`
	// Flag status after the update
	Status *string `json:"status,omitempty"`
}

type Assigner struct {
	Display *string `json:"display,omitempty"`
}

type Conditions struct {
	System  *string `json:"system,omitempty"`
	Code    *string `json:"code,omitempty"`
	Display *string `json:"display,omitempty"`
}

type DateSchema string // format: Y-m-d

// DateTimeSchema - Date and time schema in Y-m-d H:i:s format
type DateTimeSchema string

type FhirObservationCoding struct {
	Code    *string `json:"code,omitempty"`
	System  *string `json:"system,omitempty"`
	Display *string `json:"display,omitempty"`
}

type FhirUserIdentifier struct {
	Use      interface{} `json:"use,omitempty"`
	Type     Type        `json:"type,omitempty"`
	System   *string     `json:"system,omitempty"`
	Value    *string     `json:"value,omitempty"`
	Assigner Assigner    `json:"assigner,omitempty"`
}

type GeneratedApiKey struct {
	// The API Key secret value
	ApiKey interface{} `json:"apiKey"`
	// Moment that the API Key expires
	Expiration interface{} `json:"expiration"`
	// Description of the API Key
	Description interface{} `json:"description"`
}

type Group struct {
	Id   *UuidSchema `json:"id,omitempty"`
	Name *string     `json:"name,omitempty"`
}

// GroupTransformer - Details of the group.
type GroupTransformer struct {
	// The unique identifier of the group.
	Id *UuidSchema `json:"id,omitempty"`
	// The unique identifier of the organization the group belongs to.
	OrganizationId *UuidSchema `json:"organizationId,omitempty"`
	// The name of the group.
	Name *string `json:"name,omitempty"`
	// Whether the group is active.
	Active *bool `json:"active,omitempty"`
	// Whether process automatically overdue alerts.
	AutoProcessOverdue *bool `json:"autoProcessOverdue,omitempty"`
	// Time when overdue for healthcare is checked.
	OverdueCheckDailyAt interface{} `json:"overdueCheckDailyAt,omitempty"`
	// Time when action reminders for patients are sent.
	ActionsCheckDailyAt *TimeSchema `json:"actionsCheckDailyAt,omitempty"`
	// Time when overdue reminders for patients in group are sent.
	PatientsOverdueCheckDailyAt interface{} `json:"patientsOverdueCheckDailyAt,omitempty"`
	// Number of days after an alert is created for inactive patient.
	OverdueInactiveNumberOfDays *int `json:"overdueInactiveNumberOfDays,omitempty"`
	// Whether the group is a demo group.
	IsDemo *bool `json:"isDemo,omitempty"`
}

// HcpsReportResponse - Fields of user response.
type HcpsReportResponse struct {
	// The unique identifier of the user.
	UUID *UuidSchema `json:"UUID,omitempty"`
	// The first name of the user.
	FirstName *string `json:"First_Name,omitempty"`
	// The middle name of the user.
	MiddleName interface{} `json:"Middle_Name,omitempty"`
	// The last name of the user.
	LastName *string `json:"Last_Name,omitempty"`
	// The email of the user.
	Email *string `json:"Email,omitempty"`
	// The username of the user.
	UserName *string `json:"User_Name,omitempty"`
	// The language of the user.
	Language *string `json:"Language,omitempty"`
	// The role of the user.
	Role *string `json:"Role,omitempty"`
	// The status of the user.
	Status *string `json:"Status,omitempty"`
	// The date and time when the user last logged in.
	LastLoginAt *DateTimeSchema `json:"Last_Login_At,omitempty"`
	// The date and time when the user last logged out.
	LastLogoutAt *DateTimeSchema `json:"Last_Logout_At,omitempty"`
	// The groups the user belongs to.
	Groups *string `json:"Groups,omitempty"`
	// The number of processed alerts.
	ProcessedAlerts *int `json:"Processed_Alerts,omitempty"`
}

// InstrumentItemLocalisationTransformer - Details of the item.
type InstrumentItemLocalisationTransformer struct {
	// The identifier of the item this localisation is related to.
	ItemId *UuidSchema `json:"itemId,omitempty"`
	// The identifier of the item this localisation is related to.
	ItemVersion *UuidSchema `json:"itemVersion,omitempty"`
	// Locale of the item
	Locale *LocaleSchema `json:"locale,omitempty"`
	// The title of the item.
	Title interface{} `json:"title,omitempty"`
	// The subtitle of the item.
	Subtitle interface{} `json:"subtitle,omitempty"`
	// The unit of the item.
	Unit interface{} `json:"unit,omitempty"`
}

// InstrumentItemResponseLocalisationTransformer - Details of the item response.
type InstrumentItemResponseLocalisationTransformer struct {
	// The identifier of the item response this localisation is related to. The (itemResponseId, locale) pair is unique.
	ItemResponseId *UuidSchema `json:"itemResponseId,omitempty"`
	// The locale of the item response.
	Locale *LocaleSchema `json:"locale,omitempty"`
	// The text of the item response.
	ResponseText *string `json:"responseText,omitempty"`
}

// InstrumentItemResponseTransformer - Details of the item response.
type InstrumentItemResponseTransformer struct {
	// The unique identifier of the item response.
	Id *UuidSchema `json:"id,omitempty"`
	// The unique identifier of the item the response belongs to.
	ItemId *UuidSchema `json:"itemId,omitempty"`
	// The verstion identifier of the item the response belongs to.
	ItemVersion *UuidSchema `json:"itemVersion,omitempty"`
	// The score of the item response.
	Score *int `json:"score,omitempty"`
}

// InstrumentItemSetTransformer - Details of the item set.
type InstrumentItemSetTransformer struct {
	// The unique identifier of the instrument.
	InstrumentId *UuidSchema `json:"instrumentId,omitempty"`
	// The unique identifier of the item`.
	ItemId *UuidSchema `json:"itemId,omitempty"`
	// The version of the item`.
	ItemVersion *UuidSchema `json:"itemVersion,omitempty"`
}

// InstrumentItemTransformer - Details of the item.
type InstrumentItemTransformer struct {
	// The unique identifier of the item.
	Id *UuidSchema `json:"id,omitempty"`
	// The version of the item.
	Version *UuidSchema `json:"version,omitempty"`
	// The type of the moment.
	Type *string `json:"type,omitempty"`
	// The FHIR observation coding, if any.
	Coding interface{} `json:"coding,omitempty"`
	// Date and time when the item was created.
	CreatedAt *IsoDateTimeSchema `json:"createdAt,omitempty"`
	// Date and time when the item was last updated.
	UpdatedAt *IsoDateTimeSchema `json:"updatedAt,omitempty"`
}

// InstrumentLocalisationTransformer - Details of the instrument.
type InstrumentLocalisationTransformer struct {
	// The identifier of the instrument this localisation is related to.
	InstrumentId *UuidSchema `json:"instrumentId,omitempty"`
	// The version of the instrument this localisation is related to.
	InstrumentVersion *UuidSchema `json:"instrumentVersion,omitempty"`
	// Locale of the instrument
	Locale *LocaleSchema `json:"locale,omitempty"`
	// The name of the instrument.
	Name *string `json:"name,omitempty"`
	// The description of the instrument.
	Description interface{} `json:"description,omitempty"`
	// The unit of the instrument.
	Unit interface{} `json:"unit,omitempty"`
}

// InstrumentTransformer - Details of the instrument.
type InstrumentTransformer struct {
	// The identifier of the instrument. The (id, version) pair identifies uniquely an instrument.
	Id *UuidSchema `json:"id,omitempty"`
	// Version identifier of the instrument.
	Version *UuidSchema `json:"version,omitempty"`
	// The type of the instrument.
	InstrumentType *string `json:"instrumentType,omitempty"`
	// Are comments enabled for this instrument?
	CommentEnabled *bool `json:"commentEnabled,omitempty"`
	// Time when program was created.
	CreatedAt *IsoDateTimeSchema `json:"createdAt,omitempty"`
}

// IsoDateTimeSchema - Date-time formatted to ISO8601. See https://www.rfc-editor.org/rfc/rfc3339#section-5.6
type IsoDateTimeSchema string

// LocaleSchema - Two letter language identifier or locale regional country code (BCP 47)
type LocaleSchema string

// MeasurementsForQuestionnairesReportResponse - Fields of patient measurements for questionnaires response.
type MeasurementsForQuestionnairesReportResponse struct {
	// The unique identifier of the patient.
	PatientId *UuidSchema `json:"patientId,omitempty"`
	// The unique identifier of the planned action the measurement belongs to.
	PlannedActionId interface{} `json:"plannedActionId,omitempty"`
	// The unique identifier of the group the patient belongs to.
	GroupId *UuidSchema `json:"groupId,omitempty"`
	// The unique identifier of the instrument the measurement belongs to.
	InstrumentId *UuidSchema `json:"instrumentId,omitempty"`
	// The version of the instrument the measurement belongs to.
	InstrumentVersion *UuidSchema `json:"instrumentVersion,omitempty"`
	// A JSON string containing the details of the measurement.
	Details *string `json:"details,omitempty"`
	// The date and time when the measurement was created.
	MeasurementDate *DateTimeSchema `json:"measurementDate,omitempty"`
	// The unique identifier of the user who created the measurement.
	CreatedByUserId *UuidSchema `json:"createdByUserId,omitempty"`
}

// MeasurementsReportResponse - Fields of patient measurement response.
type MeasurementsReportResponse struct {
	// The unique identifier of the patient.
	PatientUUID *UuidSchema `json:"Patient_UUID,omitempty"`
	// The first name of the patient.
	FirstName *string `json:"First_Name,omitempty"`
	// The middle name of the patient.
	MiddleName interface{} `json:"Middle_Name,omitempty"`
	// The last name of the patient.
	LastName *string `json:"Last_Name,omitempty"`
	// The patient number.
	PatientNumber interface{} `json:"Patient_Number,omitempty"`
	// The group the patient belongs to.
	Group *string `json:"Group,omitempty"`
	// The program the patient is enrolled in.
	Program interface{} `json:"Program,omitempty"`
	// The protocol the patient is following.
	Protocol *string `json:"Protocol,omitempty"`
	// The date and time of the measurement.
	MeasurementDate *DateTimeSchema `json:"Measurement_Date,omitempty"`
	// The name of the measurement.
	MeasurementName *string `json:"Measurement_Name,omitempty"`
	// The value of the measurement.
	MeasurementValue *string `json:"Measurement_Value,omitempty"`
	// The flag indicating an alert.
	AlertFlag interface{} `json:"Alert_Flag,omitempty"`
	// The type of alert.
	AlertType interface{} `json:"Alert_Type,omitempty"`
	// The date and time when the alert was created.
	AlertCreatedDate interface{} `json:"Alert_Created_Date,omitempty"`
	// The type of alert processing.
	AlertProcessedType interface{} `json:"Alert_Processed_Type,omitempty"`
	// The action taken for alert processing.
	AlertProcessedAction interface{} `json:"Alert_Processed_Action,omitempty"`
	// The date and time when the alert was processed.
	AlertProcessedAt interface{} `json:"Alert_Processed_At,omitempty"`
	// The unique identifier of the user who processed the alert.
	AlertProcessedByUUID interface{} `json:"Alert_Processed_By_UUID,omitempty"`
}

// MeasurementTransformer - Measurement details
type MeasurementTransformer struct {
	// The unique identifier of the measurement. May be null if the measurement has not yet been fully processed.
	MeasurementId interface{} `json:"measurementId,omitempty"`
	// The unique identifier of the patient.
	PatientId *UuidSchema `json:"patientId,omitempty"`
	// The unique identifier of the planned action the measurement belongs to.
	PlannedActionId interface{} `json:"plannedActionId,omitempty"`
	// The unique identifier of the group the patient belongs to.
	GroupId *UuidSchema `json:"groupId,omitempty"`
	// The unique identifier of the instrument the measurement belongs to.
	InstrumentId *UuidSchema `json:"instrumentId,omitempty"`
	// The version of the instrument the measurement belongs to.
	InstrumentVersion *UuidSchema `json:"instrumentVersion,omitempty"`
	// The unique identifier of the user who created the measurement.
	CreatedByUserId *UuidSchema `json:"createdByUserId,omitempty"`
	// The date and time when the measurement was processed.
	ProcessedAt *IsoDateTimeSchema `json:"processedAt,omitempty"`
	// The date and time when the measurement was created.
	CreatedAt *IsoDateTimeSchema `json:"createdAt,omitempty"`
	// The date and time when the measurement was last updated.
	UpdatedAt *IsoDateTimeSchema `json:"updatedAt,omitempty"`
	// A JSON string containing the details of the measurement.
	Details *string `json:"details,omitempty"`
}

type Metadata struct {
	PatientId *UuidSchema `json:"patientId,omitempty"`
}

type Message struct {
	// Id of the message
	Id      *UuidSchema     `json:"id,omitempty"`
	Sender  *MessageSender  `json:"sender,omitempty"`
	Patient *MessagePatient `json:"patient,omitempty"`
	// The content of the message
	Content *string `json:"content,omitempty"`
	// Date and time when the message was read
	ReadAt interface{} `json:"readAt,omitempty"`
	// Date and time when the message was sent
	Timestamp *IsoDateTimeSchema `json:"timestamp,omitempty"`
}

// MessagePatient - The patient who the message is about
type MessagePatient struct {
	// Id of the patient
	Id *UuidSchema `json:"id,omitempty"`
	// First name of the patient
	FirstName *string `json:"firstName,omitempty"`
	// Last name of the patient
	LastName *string `json:"lastName,omitempty"`
}

// MessageSender - The sender of the message
type MessageSender struct {
	// Id of the sender
	Id interface{} `json:"id,omitempty"`
	// First name of the sender
	FirstName *string `json:"firstName,omitempty"`
	// Last name of the sender
	LastName *string `json:"lastName,omitempty"`
}

type Organization struct {
	Id         *UuidSchema `json:"id,omitempty"`
	Name       *string     `json:"name,omitempty"`
	ParentId   *UuidSchema `json:"parentId,omitempty"`
	ParentName *string     `json:"parentName,omitempty"`
}

type PaginationData struct {
	// Number of results per page
	PerPage *int `json:"perPage,omitempty"`
	// Pagination cursor to be used to fetch next results
	NextCursor interface{} `json:"nextCursor,omitempty"`
	// Url to be used to fetch next results
	NextPageUrl interface{} `json:"nextPageUrl,omitempty"`
	// Pagination cursor to be used to fetch previous results
	PrevCursor interface{} `json:"prevCursor,omitempty"`
	// Url to be used to fetch previous results
	PrevPageUrl interface{} `json:"prevPageUrl,omitempty"`
}

// PatientCreationInput - Supported fields for patient creation
type PatientCreationInput struct {
	// Patient account name
	AccountName *string `json:"accountName,omitempty"`
	// Patient email address
	Email *string `json:"email,omitempty"`
	// Patient number (identifier for the patient, will result in a FHIR identifier (PI))
	PatientNumber interface{} `json:"patientNumber,omitempty"`
	// UUID of the organization the patient belongs to
	OrganizationId *string `json:"organizationId,omitempty"`
	// UUID of the group the patient belongs to
	GroupId *string `json:"groupId,omitempty"`
	// Program UUID
	ProgramId *string `json:"programId,omitempty"`
	// Protocol UUID
	ProtocolId *string `json:"protocolId,omitempty"`
	// Enable OTP login via SMS
	SmsLoginEnabled interface{} `json:"smsLoginEnabled,omitempty"`
	// Time zone (defaults to CET)
	Timezone interface{} `json:"timezone,omitempty"`
	// Language in two-letter ISO code
	Language interface{} `json:"language,omitempty"`
	// Additional notes or metadata
	Comments interface{} `json:"comments,omitempty"`
	// The patient's first name.
	FirstName *string `json:"firstName,omitempty"`
	// The patient's middle name, if any.
	MiddleName interface{} `json:"middleName,omitempty"`
	// The patient's last name.
	LastName *string `json:"lastName,omitempty"`
	// Date of birth
	DateOfBirth interface{} `json:"dateOfBirth,omitempty"`
	// Phone number in E.164 format
	PhoneNumber *string `json:"phoneNumber,omitempty"`
	// Patient sex (male or female)
	Sex *string `json:"sex,omitempty"`
	// ISO-3166-1 country code
	AddressCountry interface{} `json:"addressCountry,omitempty"`
	// City name
	AddressCity interface{} `json:"addressCity,omitempty"`
	// Postal code
	AddressPostalCode interface{} `json:"addressPostalCode,omitempty"`
	// Street name
	AddressStreet interface{} `json:"addressStreet,omitempty"`
	// House number
	AddressHouseNumber interface{} `json:"addressHouseNumber,omitempty"`
	// House number suffix
	AddressHouseNumberSuffix interface{} `json:"addressHouseNumberSuffix,omitempty"`
	// The patient's identifiers in FHIR format.
	Identifiers []*FhirUserIdentifier `json:"identifiers,omitempty"`
}

type PatientHcpNoteBaseFieldsModel struct {
	// Healthcare provider note uuid
	Id UuidSchema `json:"id"`
	// Note title
	Title string `json:"title"`
	// Note content
	Content string `json:"content"`
	// Is the note pinned?
	IsPinned bool `json:"isPinned"`
	// Is the note kept when the patient changes program?
	IsPersistent bool `json:"isPersistent"`
	// Date and time when the note was created
	CreatedAt IsoDateTimeSchema `json:"createdAt"`
	// Date and time when the note was last updated
	UpdatedAt IsoDateTimeSchema `json:"updatedAt"`
}

// PatientsMomentsReportResponse - Fields of patientsMoments report type.
type PatientsMomentsReportResponse struct {
	// The unique identifier of the patient.
	PatientId *UuidSchema `json:"patientId,omitempty"`
	// The unique identifier of the program moment.
	MomentId *UuidSchema `json:"momentId,omitempty"`
	// The date the moment happened.
	Date *DateSchema `json:"date,omitempty"`
	// The date and time when the patient moment was created.
	CreatedAt *IsoDateTimeSchema `json:"createdAt,omitempty"`
}

// PatientsMutationsReportResponse - Fields of patientsMutations report type.
type PatientsMutationsReportResponse struct {
	// The unique identifier of the patient.
	PatientUUID *UuidSchema `json:"Patient_UUID,omitempty"`
	// The first name of the patient.
	FirstName *string `json:"First_Name,omitempty"`
	// The middle name of the patient.
	MiddleName interface{} `json:"Middle_Name,omitempty"`
	// The last name of the patient.
	LastName *string `json:"Last_Name,omitempty"`
	// The patient number.
	PatientNumber interface{} `json:"Patient_Number,omitempty"`
	// The type of mutation.
	Type *string `json:"Type,omitempty"`
	// The name of the program or group associated with the mutation.
	Name *string `json:"Name,omitempty"`
	// The date and time of the mutation.
	MutationDate *DateTimeSchema `json:"Mutation_Date,omitempty"`
}

// PatientsProgramsHistoryReportResponse - Fields of patientsProgramsHistory report type.
type PatientsProgramsHistoryReportResponse struct {
	// The unique identifier of the patient.
	PatientId *UuidSchema `json:"patientId,omitempty"`
	// The unique identifier of the program.
	ProgramId *UuidSchema `json:"programId,omitempty"`
	// The unique identifier of the user who assigned the patient to the program.
	CreatedBy *UuidSchema `json:"createdBy,omitempty"`
	// The date and time when the patient was assigned to the program.
	CreatedAt *IsoDateTimeSchema `json:"createdAt,omitempty"`
}

// PatientsReportResponse - Fields of patients response.
type PatientsReportResponse struct {
	// The unique identifier of the patient.
	PatientUUID *UuidSchema `json:"Patient_UUID,omitempty"`
	// The first name of the patient.
	FirstName *string `json:"First_Name,omitempty"`
	// The middle name of the patient.
	MiddleName interface{} `json:"Middle_Name,omitempty"`
	// The last name of the patient.
	LastName *string `json:"Last_Name,omitempty"`
	// The sex of the patient.
	Sex *string `json:"Sex,omitempty"`
	// The patient number.
	PatientNumber interface{} `json:"Patient_Number,omitempty"`
	// The BSN of the patient.
	BSN interface{} `json:"BSN,omitempty"`
	// The email of the patient.
	Email *string `json:"Email,omitempty"`
	// The phone number of the patient.
	Phone interface{} `json:"Phone,omitempty"`
	// The date of birth of the patient.
	DateOfBirth *DateTimeSchema `json:"Date_Of_Birth,omitempty"`
	// The status of the patient.
	Status *string `json:"Status,omitempty"`
	// The name of the group.
	GroupName *string `json:"Group_Name,omitempty"`
	// The name of the program.
	ProgramName *string `json:"Program_Name,omitempty"`
	// The name of the protocol.
	Protocol *string `json:"Protocol,omitempty"`
	// The date and time when the patient was created.
	CreatedDate *DateTimeSchema `json:"Created_Date,omitempty"`
	// The date and time when the patient was activated.
	ActivatedDate *DateTimeSchema `json:"Activated_Date,omitempty"`
	// The date and time when the patient was stopped.
	StoppedDate *DateTimeSchema `json:"Stopped_Date,omitempty"`
	// The reason for stopping the patient.
	StoppedReason interface{} `json:"Stopped_Reason,omitempty"`
	// The number of grey alerts.
	GreyAlerts *string `json:"Grey_Alerts,omitempty"`
	// The number of orange alerts.
	OrangeAlerts *string `json:"Orange_Alerts,omitempty"`
	// The number of red alerts.
	RedAlerts *string `json:"Red_Alerts,omitempty"`
	// The number of measurement alerts.
	MeasurementsAlerts *string `json:"Measurements_Alerts,omitempty"`
	// The number of combination alerts.
	CombinationAlerts *string `json:"Combination_Alerts,omitempty"`
	// The number of remark alerts.
	RemarkAlerts *string `json:"Remark_Alerts,omitempty"`
	// The number of overdue alerts.
	OverduesAlerts *string `json:"Overdues_Alerts,omitempty"`
	// The number of average alerts.
	AverageAlerts *string `json:"Average_Alerts,omitempty"`
	// The total number of alerts.
	TotalAlerts *int `json:"Total_Alerts,omitempty"`
}

// PatientsSettingsHistoryReportResponse - Fields of patientsSettingsHistory report type.
type PatientsSettingsHistoryReportResponse struct {
	// The unique identifier of the patient.
	PatientId *UuidSchema `json:"patientId,omitempty"`
	// The name of the setting.
	Key *string `json:"key,omitempty"`
	// The value of the setting.
	Value *string `json:"value,omitempty"`
	// The unique identifier of the user who changed the setting.
	CreatedBy *UuidSchema `json:"createdBy,omitempty"`
	// The date and time when the setting was created.
	CreatedAt *IsoDateTimeSchema `json:"createdAt,omitempty"`
}

// PatientsWorkflowHistoryReportResponse - Fields of patientsWorkflowHistory report type.
type PatientsWorkflowHistoryReportResponse struct {
	// The unique identifier of the patient.
	PatientId *UuidSchema `json:"patientId,omitempty"`
	// The unique identifier of the workflow.
	WorkflowId *UuidSchema `json:"workflowId,omitempty"`
	// The date when the workflow was triggered.
	TriggerDay *DateSchema `json:"triggerDay,omitempty"`
	// The number of times the workflow was triggered.
	TriggerCount *int `json:"triggerCount,omitempty"`
	// The date and time when the workflow was created.
	CreatedAt *IsoDateTimeSchema `json:"createdAt,omitempty"`
}

// PatientTransformer - Patient details
type PatientTransformer struct {
	// The unique identifier of the patient.
	Id *UuidSchema `json:"id,omitempty"`
	// The date and time when the patient was created.
	CreatedAt *IsoDateTimeSchema `json:"createdAt,omitempty"`
	// The date and time when the patient was last updated.
	UpdatedAt *IsoDateTimeSchema `json:"updatedAt,omitempty"`
	// The date when the patient was activated.
	ActivatedAt interface{} `json:"activatedAt,omitempty"`
	// Patient's timezone.
	Timezone *TimezoneIdSchema `json:"timezone,omitempty"`
	// Patient's language.
	Language *string `json:"language,omitempty"`
	// Patient's first name.
	FirstName *string `json:"firstName,omitempty"`
	// Patient's middle name.
	MiddleName interface{} `json:"middleName,omitempty"`
	// Patient's last name.
	LastName *string `json:"lastName,omitempty"`
	// Patient's date of birth.
	DateOfBirth interface{} `json:"dateOfBirth,omitempty"`
	// Patient's gender.
	Sex *string `json:"sex,omitempty"`
	// Patient's status.
	Status *string `json:"status,omitempty"`
	// Patient's email address.
	Email interface{} `json:"email,omitempty"`
	// Patient's phone number.
	Phone interface{} `json:"phone,omitempty"`
	// Patient's account name.
	AccountName interface{} `json:"accountName,omitempty"`
	// Patient's identifiers.
	Identifiers  []*FhirUserIdentifier `json:"identifiers,omitempty"`
	Organization Organization          `json:"organization,omitempty"`
	Group        Group                 `json:"group,omitempty"`
	Program      Program               `json:"program,omitempty"`
	Protocol     Protocol              `json:"protocol,omitempty"`
}

// PlannedActionsReportResponse - Fields of plannedActions report type.
type PlannedActionsReportResponse struct {
	// The unique identifier of the planned action.
	Id *UuidSchema `json:"id,omitempty"`
	// The unique identifier of the program action.
	ProgramActionId *UuidSchema `json:"programActionId,omitempty"`
	// The unique identifier of the patient.
	PatientId *UuidSchema `json:"patientId,omitempty"`
	// The method of planning the action. Whether rec for frequency-based recalculation routine or wfa for workflow actions.
	PlannedVia *string `json:"plannedVia,omitempty"`
	// Whether the action was planned by the patient themselves.
	SelfPlanned *int `json:"selfPlanned,omitempty"`
	// The date and time after which the action can only be completed.
	NotBefore *IsoDateTimeSchema `json:"notBefore,omitempty"`
	// The date and time after which the action can no longer be completed.
	ExpiresAt *IsoDateTimeSchema `json:"expiresAt,omitempty"`
	// The timezone of the notBefore and expiresAt fields as a region string.
	ValidPeriodTimezone *string `json:"validPeriodTimezone,omitempty"`
	// The date and time when the action was completed.
	CompletedAt *IsoDateTimeSchema `json:"completedAt,omitempty"`
	// The timezone of the completedAt field as a region string.
	CompletedAtTimezone *string `json:"completedAtTimezone,omitempty"`
	// The date and time when the action was created.
	CreatedAt *IsoDateTimeSchema `json:"createdAt,omitempty"`
}

// ProgramActionItemTransformer - Details of the program action item.
type ProgramActionItemTransformer struct {
	// The identifier of the related program action. The (actionId, instrumentId, careBlockId) together identifies an item uniquely.
	ActionId interface{} `json:"actionId,omitempty"`
	// The identifier of the related instrument. The (actionId, instrumentId, careBlockId) together identifies an item uniquely.
	InstrumentId interface{} `json:"instrumentId,omitempty"`
	// The identifier of the related patient information (care block). The (actionId, instrumentId, careBlockId) together identifies an item uniquely.
	CareBlockId interface{} `json:"careBlockId,omitempty"`
}

// ProgramActionLocalisationTransformer - Details of the action localisation.
type ProgramActionLocalisationTransformer struct {
	// The identifier of the program action item this localisation is related to. The (actionId, locale) pair is unique.
	ActionId *UuidSchema `json:"actionId,omitempty"`
	// Locale of the program action item.
	Locale *LocaleSchema `json:"locale,omitempty"`
	// The name of the program action item.
	Name *string `json:"name,omitempty"`
	// Date and time when the localisation was created.
	CreatedAt *IsoDateTimeSchema `json:"createdAt,omitempty"`
}

// ProgramActionTransformer - Details of the program action.
type ProgramActionTransformer struct {
	// The unique identifier of the program action.
	Id *UuidSchema `json:"id,omitempty"`
	// The unique identifier of the related program.
	ProgramId *UuidSchema `json:"programId,omitempty"`
	// Program action category
	Category interface{} `json:"category,omitempty"`
	// Whether the action should be shown only when it is planned or also should be available from self-care as optional action
	OnlyWhenPlanned *bool `json:"onlyWhenPlanned,omitempty"`
	// Date and time when the program action was created.
	CreatedAt *IsoDateTimeSchema `json:"createdAt,omitempty"`
	// Date and time when the program action was soft deleted.
	DeletedAt interface{} `json:"deletedAt,omitempty"`
}

// ProgramMomentLocalisationTransformer - Details of the moment localisation.
type ProgramMomentLocalisationTransformer struct {
	// The unique identifier of the moment the localisation belongs to.
	MomentId *UuidSchema `json:"momentId,omitempty"`
	// The name of the localisation.
	Name *string `json:"name,omitempty"`
	// The description of the localisation.
	Description *string       `json:"description,omitempty"`
	Locale      *LocaleSchema `json:"locale,omitempty"`
}

// ProgramMomentTransformer - Details of the moment.
type ProgramMomentTransformer struct {
	// The unique identifier of the moment.
	Id *UuidSchema `json:"id,omitempty"`
	// The unique identifier of the program the moments belongs to.
	ProgramId *UuidSchema `json:"programId,omitempty"`
	// The type of the moment.
	Type *string `json:"type,omitempty"`
	// The name of the moment.
	Name *string `json:"name,omitempty"`
	// The access rights of the moment.
	PatientAccess *string `json:"patientAccess,omitempty"`
	// Date and time when the moment was created.
	CreatedAt *IsoDateTimeSchema `json:"createdAt,omitempty"`
}

// ProgramTransformer - Details of the program.
type ProgramTransformer struct {
	// The unique identifier of the program.
	Id *UuidSchema `json:"id,omitempty"`
	// The name of the program.
	Name *string `json:"name,omitempty"`
	// The information url of the program.
	InformationUrl *string `json:"informationUrl,omitempty"`
	// Whether two way messaging is enabled.
	IsTwoWayMessagingEnabled *bool `json:"isTwoWayMessagingEnabled,omitempty"`
	// FHIR conditions associated with the program.
	Conditions []Conditions `json:"conditions,omitempty"`
	// Date and time when program was created.
	CreatedAt *IsoDateTimeSchema `json:"createdAt,omitempty"`
}

type Program struct {
	Id        *UuidSchema `json:"id,omitempty"`
	Name      *string     `json:"name,omitempty"`
	StartDate *DateSchema `json:"startDate,omitempty"`
	EndDate   *DateSchema `json:"endDate,omitempty"`
}

type Protocol struct {
	Id   *UuidSchema `json:"id,omitempty"`
	Name *string     `json:"name,omitempty"`
}

// ProtocolTransformer - Details of the protocol.
type ProtocolTransformer struct {
	// The unique identifier of the protocol.
	Id *UuidSchema `json:"id,omitempty"`
	// The unique identifier of the program the protocol belongs to.
	ProgramId *UuidSchema `json:"programId,omitempty"`
	// The name of the protocol.
	Name *string `json:"name,omitempty"`
	// The description of the protocol.
	Description interface{} `json:"description,omitempty"`
	// The reminder time for patient actions.
	PatientActionReminderTime interface{} `json:"patientActionReminderTime,omitempty"`
	// The overdue reminder time for healthcare.
	HealthcareOverdueReminderTime interface{} `json:"healthcareOverdueReminderTime,omitempty"`
	// The overdue reminder time for patients.
	PatientOverdueReminderTime interface{} `json:"patientOverdueReminderTime,omitempty"`
	// The timezone of the protocol.
	Timezone *TimezoneIdSchema `json:"timezone,omitempty"`
	// Whether the protocol auto-processes overdue items.
	AutoProcessOverdues *bool `json:"autoProcessOverdues,omitempty"`
	// The limit for inactivity overdue.
	InactivityOverdueLimit *int `json:"inactivityOverdueLimit,omitempty"`
	// Whether the protocol triggers comment alerts.
	TriggerCommentAlerts *bool `json:"triggerCommentAlerts,omitempty"`
	// The creation date of the protocol.
	CreatedAt interface{} `json:"createdAt,omitempty"`
}

type PublicV1ErrorElement struct {
	// Machine-readable designation of the error
	Code string `json:"code"`
	// Human-readable designation of the error, intended for developers, not necessarily end-users
	Message string `json:"message"`
	// Name of the attribute for which the validation failed, only in case of validation errors
	Attribute *string `json:"attribute,omitempty"`
}

type PublicV1ResponseBody struct {
	// Representation of the resource or collection of resources targeted by a request
	Data map[string]interface{} `json:"data,omitempty"`
	// Additional information about problems encountered while performing an operation
	Errors []*PublicV1ErrorElement `json:"errors,omitempty"`
	// Non-standard meta-information
	Meta map[string]interface{} `json:"meta,omitempty"`
}

// StarRatingsReportResponse - Fields of patient score response.
type StarRatingsReportResponse struct {
	// The group the patient belongs to.
	Group *string `json:"Group,omitempty"`
	// The date and time of the score.
	Date *DateTimeSchema `json:"Date,omitempty"`
	// The unique identifier of the patient.
	PatientUUID *UuidSchema `json:"Patient_UUID,omitempty"`
	// The score of the patient.
	Score *string `json:"Score,omitempty"`
	// Additional remarks.
	Remarks interface{} `json:"Remarks,omitempty"`
}

// TimeSchema - Time schema in H:i:s format
type TimeSchema string

type TimezoneIdSchema string

type FhirIdentifierTypeCoding struct {
	System *string `json:"system,omitempty"`
	Code   *string `json:"code,omitempty"`
}

type Type struct {
	Coding []*FhirIdentifierTypeCoding `json:"coding,omitempty"`
}

// UsersStatusChangeReasonsReportResponse - Fields of usersStatusChangeReasons report type.
type UsersStatusChangeReasonsReportResponse struct {
	// The unique identifier of the user.
	UserId *UuidSchema `json:"userId,omitempty"`
	// The status the user was in before the change.
	FromStatus *string `json:"fromStatus,omitempty"`
	// The status the user was in after the change.
	ToStatus *string `json:"toStatus,omitempty"`
	// The reason for the status change.
	Reason *string `json:"reason,omitempty"`
	// The unique identifier of the user who changed the status.
	CreatedBy *UuidSchema `json:"createdBy,omitempty"`
	// The date and time when the status change was created.
	CreatedAt *IsoDateTimeSchema `json:"createdAt,omitempty"`
	// The date and time when the status change was processed.
	ProcessedAt *IsoDateTimeSchema `json:"processedAt,omitempty"`
}

type UuidSchema string

// WebhooksEventsDetailTransformer - Webhook event
type WebhooksEventsDetailTransformer struct {
}

// WebhooksEventsTransformer - Webhook event
type WebhooksEventsTransformer struct {
	// Webhook event identifier
	Id *string `json:"id,omitempty"`
	// Status of the webhook event
	Status interface{} `json:"status,omitempty"`
	// Headers being used to send the event
	Headers interface{} `json:"headers,omitempty"`
	// The event type of the webhook event
	EventType interface{} `json:"eventType,omitempty"`
	// The payload of the webhook event when being sent
	Payload *string `json:"payload,omitempty"`
	// Name of the entity related to the event
	RelatedEntityName interface{} `json:"relatedEntityName,omitempty"`
	// Metadata attached to the event
	Metadata Metadata `json:"metadata,omitempty"`
	// Name of the subscriber of the event
	SubscriberName *string `json:"subscriberName,omitempty"`
	// Receiver uri of the webhook event
	Uri *string `json:"uri,omitempty"`
	// The format of the webhook event
	Format *string `json:"format,omitempty"`
	// The creation date of the webhook event
	CreatedAt *IsoDateTimeSchema `json:"createdAt,omitempty"`
	// The last update date of the webhook event
	UpdatedAt *IsoDateTimeSchema `json:"updatedAt,omitempty"`
}
