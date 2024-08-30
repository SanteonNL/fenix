# Software Requirement Specification (SRS)

## Introduction

An SRS introduction is exactly what you expect—it’s a 10,000-foot view of the overall project. When writing your introduction, describe the purpose of the product, the intended audience, and how the audience will use it. In your introduction, make sure to include:

- Product scope: The scope should relate to the overall business goals of the product, which is especially important if multiple teams or contractors will have access to the document. List the benefits, objectives, and goals intended for the product.

- Product value: Why is your product important? How will it help your intended audience? What function will it serve, or what problem will it solve? Ask yourself how your audience will find value in the product.

- Intended audience: Describe your ideal audience. They will dictate the look and feel of your product and how you market it.

- Intended use: Imagine how your audience will use your product. List the functions you provide and all the possible ways your audience can use your product depending on their role. It’s also good practice to include use cases to illustrate your vision.

- Definitions and acronyms: Every industry or business has its own unique acronyms or jargon. Lay out the definitions of the terms you are using in your SRS to ensure all parties understand what you’re trying to say.

- Table of contents: A thorough SRS document will likely be very long. Include a table of contents to help all participants find exactly what they’re looking for.

Make sure your introduction is clear and concise. Remember that your introduction will be your guide to the rest of the SRS outline, and you want it to be interpreted the same by everyone using the doc.

## Functional requirements

Once you have your introduction, it’s time to get more specific. Functional requirements break down system features and functions that allow your system to perform as intended.

Use your overview as a reference to check that your requirements meet the user’s basic needs as you fill in the details. There are thousands of functional requirements to include depending on your product. Some of the most common are:

- If/then behaviors

- Data handling logic

- System workflows

- Transaction handling

- Administrative functions

- Regulatory and compliance needs

- Performance requirements

- Details of operations conducted for every screen

If this feels like a lot, try taking it one requirement at a time. The more detail you can include in your SRS document, the less troubleshooting you’ll need to do later on.

## External interface requirements

External interface requirements are types of functional requirements that ensure the system will communicate properly with external components, such as:

- User interfaces: The key to application usability that includes content presentation, application navigation, and user assistance, among other components.

- Hardware interfaces: The characteristics of each interface between the software and hardware components of the system, such as supported device types and communication protocols.  

- Software interfaces: The connections between your product and other software components, including databases, libraries, and operating systems.

- Communication interfaces: The requirements for the communication functions your product will use, like emails or embedded forms.

Embedded systems rely on external interface requirements. You should include things like screen layouts, button functions, and a description of how your product depends on other systems.

## Non-functional requirements

The final section of your SRS details non-functional requirements. While functional requirements tell a system what to do, non-functional requirements (NFRs) determine how your system will implement these features. For example, a functional requirement might tell your system to print a packing slip when a customer orders your product. An NFR will ensure that the packing slip prints on 4”x6” white paper, the standard size for packing slips.

While a system can still work if you don’t meet NFRs, you may be putting user or stakeholder expectations at risk. These requirements keep functional requirements in check, so it still includes attributes like product affordability and ease of use.

The most common types of NFRs are called the ‘Itys’. They are:

- Security: What’s needed to ensure any sensitive information your software collects from users is protected.

- Capacity: Your product’s current and future storage needs, including a plan for how your system will scale up for increasing volume demands.

- Compatibility: The minimum hardware requirements for your software, such as support for operating systems and their versions.

- Reliability and availability: How often you expect users to be using your software and what the critical failure time is under normal usage.

- Scalability: The highest workloads under which your system will still perform as expected.

- Maintainability: How your application should use continuous integration so you can quickly deploy features and bug fixes.

- Usability: How easy it is to use the product.

Other common types of non-functional requirements include performance, regulatory, and environmental requirements.

Source: <https://asana.com/nl/resources/software-requirement-document-template>
