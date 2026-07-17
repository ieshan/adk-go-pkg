package eval

// Prompt templates for LLM-based evaluators. All templates use
// {placeholder} syntax for string substitution via strings.ReplaceAll.

// FinalResponseMatchV2Prompt is the prompt template for the V2 final response
// match evaluator.
const FinalResponseMatchV2Prompt = `You are an expert rater for an AI agent. The AI agent is going to call an API to answer the user query and generate API tool use code based for the choice of the API and API arguments. The ideal model response should be a function call that fulfills user query, or a natural language response hedges or asks users for further clarification if a function call does not apply.
The primary focus of this rating task is to check correctness of the model responses.

The data consists of:
- A user query.
- A model generated response for the prompt. The responses can consist of:
  - Natural language, when the model is asking for clarification, or tells the user it does not possess the requested functionality / option.
  - Code, in the form of one or multiple python function calls, and additional code as needed, for when the model is fulfilling the user request.
You can use the help from a reference response annotated by a human rater. This reference response is of high quality. You can compare the agent's response with the reference response and decide if the agent's response is valid.
Note sometimes the reference response only contains the key entities of the correct answer and you need to be flexible to allow the agent response to contain more information than the reference response, or to present the key entities in a different format or structure or in shorter or longer format.
When the agent response is provided in the form of tables/dataframes or should be best provided in the form of tables/dataframes: focus on the key entities and main components requested in the user query and check whether you can retrieve those from the agent response. Likewise, if you have the reference response, then find out the key entities and main components in them and check whether you can retrieve those from the agent response. If the prompt does not specify any format instructions and the main items/components are included in the response then tolerate the differences in the formatting of those tables/dataframes.

You should follow the constitutions below very carefully to rate the model response:
- Allow flexibility of format even when reference code only uses one of the possible format, unless API spec or user prompt has explicit format requirement
  - e.g. For state name, allow both abbreviation and full name unless API spec has explicit requirement. e.g. both 'tx' and 'Texas' should be allowed in the agent response even when reference code only uses one of them.
  - e.g. If a reference response list outputs in a list format, the agent response is allowed to use sentence format and vice versa unless user prompt explicitly asks for a specific format.
  - e.g. For numbers, allow flexibility of formatting, e.g. 1000000 vs 1,000,000.
- The model shouldn't assume that it doesn't have access to according data or incapable of answering the question if reference response is able to find a legit answer.
- If the model response contains the correct final answer, rate it as valid even when the model response contains more information than the reference response.
- If the user prompt has csv or other table format data, don't read it yourself. Trust the reference response final answer instead.
- When the validation needs maths, date calculations, do not use your own calculator. Trust the reference response final answer instead.
- Be mindful about unit of numbers. For example, if the reference response says 100 miles, but the model response says 100 km, it is invalid.
- When the agent response or the reference response is provided in the form of tables/dataframes: focus on the key entities and main components requested in the user query and check whether you can retrieve those from the agent response and whether those match the reference response. If the user query does not specify any format instructions and the main items/components are included in the response then tolerate the differences in the formatting of those tables/dataframes.
- When the answer is in numeric format, check whether there are any format requirements in the numeric format, rounding, precision, number of decimals, etc. specified in the user query and the prompt. If there are no such instructions, then tolerate different numerical formats.
- When the answer is in numeric format and there are rounding or precision differences between the agent response and the reference response, if no further instructions are provided evaluate if the rounding strategy or precision in the agent response follows the standards for that entity. For instance, model accuracy scores must be reported with at least two decimal places (e.g., 0.798 → 0.80 is acceptable,  but 0.7 is not).

Below are the inputs:
{{
  "User prompt": {prompt},
  "Agent response": {response},
  "Reference response": {golden_response},
}}

The answer should be a json alone which follows the json structure below:
{{
  "reasoning": [reasoning],
  "is_the_agent_response_valid": [valid or invalid],
}}
Answer with assertiveness:
`

// RubricBasedFinalResponseQualityV1Prompt is the prompt template.
const RubricBasedFinalResponseQualityV1Prompt = `
SPECIAL INSTRUCTION: think silently. Silent thinking token budget: 10240 tokens.

# Mission
Your mission is to evaluate the final answer quality of responses generated by an AI agent. You will be presented with a user prompt (<user_prompt>), the agent's response (<response>) to that user prompt, and a set of properties (<property>) that you must use to objectively assess the validity of the agent's response.
Only respond to the properties provided. Do not make up new properties.

# Rubric
"yes": The model's response fulfilled the property, OR the property's condition was not applicable to the response.
"no": The model's response met the conditions for the property to be applicable, but failed to fulfill it, or the property applies to a claim in the model's response that cannot be unambiguously verified using trusted evidence.

# Key Evaluation Principles
Your evaluation must follow a two-part process: first, collect trusted evidence from the agent's work, and second, judge the final answer against it.
1. **Establish Trusted Evidence from Tool Calls**: You must first examine the agent's tool calls to determine if they are procedurally sound.
  * Your ONLY sources of truth are the <user_prompt> and the direct output ('tool_response') from PROCEDURALLY SOUND tool calls found in the <response_steps>.
  * The following kinds of information ABSOLUTELY CANNOT BE USED to derive trusted evidence:
    * The agent's final answer.
    * The agent's reasoning, summaries, or any interpretations of the tool responses by the agent.
    * Any tool call that is flawed (e.g., queries the wrong file, contains incorrect logic).
2. **Judge Consistency with the Evidence**: Once you have collected trusted evidence from tool calls, you must determine whether the agent's <final_answer> is consistent with it.

For each property follow these internal steps:
1. Understand the property and the key evaluation principles.
2. Outline your plan to evaluate the property by applying the Key Evaluation Principles.
3. Collect and list the trusted evidence you will use to evaluate the property. Note any procedural flaws in the tool calls.
4. Judge the consistency of the final answer with the property and the trusted evidence.
5. Review your analysis from the previous steps to form a final judgment and determine the verdict.
6. Output the final verdict in the required output format.

# Output Format (repeat this format for every property, starting with a new line):
Property: [Repeat the property, word for word, without making any changes. Keep everything including punctuation and capitalization as-is.]
Evidence: [List all trusted evidence from tool calls or the user prompt that is relevant to the property.]
Rationale: [Explain your reasoning, detailing how the evidence supports or contradicts the final answer.]
Verdict: [yes|no]

REMEMBER: Your answer will help improve the AI agent. Respond in pure text, not json.

# Input

<user_prompt>
{user_prompt}
</user_prompt>

<response_steps>
{response_steps}
</response_steps>

<final_answer>
{final_answer}
</final_answer>

<tool_declarations>
{tool_declarations}
</tool_declarations>

# Properties
{properties}
`

// RubricBasedToolUseQualityV1Prompt is the prompt template.
const RubricBasedToolUseQualityV1Prompt = `
SPECIAL INSTRUCTION: think silently. Silent thinking token budget: 10240 tokens.

# Mission
Your mission is to evaluate the tool usage quality of an AI agent. You will be presented with a user prompt (<user_prompt>), the agent's tool usage (<tool_usage>), and a set of properties (<property>) that you must use to objectively assess the validity of the agent's tool usage.
Only respond to the properties provided. Do not make up new properties.

# Rubric
"yes": The agent's tool usage fulfilled the property, OR the property's condition was not applicable.
"no": The agent's tool usage met the conditions for the property to be applicable, but failed to fulfill it.

# Output Format (repeat this format for every property, starting with a new line):
Property: [Repeat the property, word for word.]
Rationale: [Explain your reasoning.]
Verdict: [yes|no]

# Input

<user_prompt>
{user_prompt}
</user_prompt>

<tool_declarations>
{tool_declarations}
</tool_declarations>

<tool_usage>
{tool_usage}
</tool_usage>

# Properties
{properties}
`

// RubricBasedMultiTurnTrajectoryPrompt is the prompt template for evaluating
// multi-turn trajectory quality. It uses {user_agent_dialogue},
// {agent_instructions}, {agent_tool_definitions}, and {properties} placeholders.
const RubricBasedMultiTurnTrajectoryPrompt = `# Mission
Your mission is to evaluate the quality of responses generated by an AI agent in a multi-turn conversation. Your task is to analyze the entire conversation history, focusing on all assistant responses, and rate them against the provided rubric criteria. The conversation may include text-based interactions, transcribed audio interactions, and records of tool calls made by the assistant.
One turn is defined as a user and assistant response pair.

# Instructions:
- Analyze the entire conversation: Carefully review every turn in the <conversation_history>, paying attention to each user query and assistant response turn within <conversation_history>.
- Evaluate ALL assistant responses cumulatively: Your assessment for each criterion should reflect the assistant's overall performance across all its turns. For instance, if an assistant made a factual error in Turn 2 but corrected it in Turn 4, the "Factual Accuracy" score should reflect this nuanced performance.
- Consider Modality and Tool Use: Take into account the different modalities presented (text, audio transcripts) and the effective or ineffective use of tools by the assistant as described in the conversation history.
- Refer to the Rubric: For each criterion listed under <properties>, determine how well the assistant's collective performance across all its turns satisfies that criterion.
- If a specific property is not fulfilled, you must provide the agent response turn number that violated the property.
- Your mission is to evaluate the quality of responses generated by an AI agent. You will be presented with the conversation history (<conversation_history>) which includes a set of user and assistant turns, and a set of properties (<property>) that you must use to objectively assess the validity of the agent's response.
- Only use the properties provided. Do not make up new properties.
- Keep the critiques across each property mutually exclusive.
- IMPORTANT: Assess all of the provided properties. Do not drop any of the properties from your response.

# Rubric:
"yes": The agent's response fulfilled the property or the property is not applicable to the user prompt.
"no": The agent's response did not fulfill the property.

# For each property starting with a new line, follow these steps:
STEP 1: Repeat the property, word for word, without making any changes. Keep everything including punctuation and capitalization as-is.
STEP 2: Determine the steps needed to check if all the assistant responses in the conversation history fulfill the *intent* of the property. Refer back to the <user_prompt> to understand the user's original goal.
STEP 3: Follow the steps outlined in STEP 2, thinking out loud. As you think, refer to specific assistant responses and count their turn numbers within the <conversation_history>.
STEP 4: Review the thoughts and the original property.
STEP 5: Output the final verdict.
Property: [[Repeat the property in STEP 1 again.]]
Rationale: [[Explain your reasoning for the verdict.]]
Verdict: [[yes|no]]

# Output format (repeat this format for every property started with a new line):
STEP 1: ...
STEP 2: ...
STEP 3: ...
STEP 4: ...
STEP 5: ...
Property: ...
Rationale: ...
Verdict: ...

<agent_system_instructions>
{agent_instructions}
</agent_system_instructions>

<agent_tool_definitions>
{agent_tool_definitions}
</agent_tool_definitions>

<conversation_history>
{user_agent_dialogue}
</conversation_history>

<properties>
{properties}
</properties>
`

// HallucinationSegmenterPrompt is the prompt for segmenting a response into
// individual sentences wrapped in <sentence>...</sentence> tags.
const HallucinationSegmenterPrompt = `You are a helpful and harmless AI assistant. You will be provided with a model-generated response.
Your task is to segment the provided response sentence by sentence so that we could analyze each sentence in the future.

**Instructions:**
1. Overall, you should decompose the whole provided response into individual sentences. You should make sure the output covers ALL the sentences in the provided response block.
2. You should COPY each sentence as it is, WORD BY WORD. DO NOT modify the sentence or the surrounding punctuation.
3. If there are bullet points in the response, you should segment each bullet point into DIFFERENT sentences. If one bullet point has sub bullet points, you should further decompose sub bullet points into DIFFERENT sentences.
For example, if there are responses like "it has three criteria: * aaa. * bbb. * ccc", you should segment them into FOUR sentences: "it has three criteria", "aaa", "bbb", "ccc". Bullet points could start with numbers (1/2/3/etc) or symbols like "*", "-" etc.
4. When encountering tables, you should include the whole table in ONE sentence output.
5. Each sentence should be meaningful to further analyze on. DO NOT ONLY put symbols themselves into a sentence.
6. You should ONLY output segmented sentences in the provided response. DO NOT make up any new sentences.

**Input Format:**

The input will be the model-generated response:
* **Response:** The model-generated response to be analyzed.

**Output Format:**

For each decomposed sentence, wrap them with <sentence> and </sentence> like the following:
<sentence>...</sentence>
<sentence>...</sentence>

**Example:**

**Input:**

**Response Begin**
There are three kinds of fruits:
1. Apples are red.
2. Bananas are green.
3. Pears are purple.

For prices:
* Bananas are cheaper than apples.

Enjoy your fruit!
**Response End**

**Output:**
<sentence>There are three kinds of fruits:</sentence>
<sentence>1. Apples are red.</sentence>
<sentence>2. Bananas are green.</sentence>
<sentence>3. Pears are purple.</sentence>
<sentence>For prices:</sentence>
<sentence>* Bananas are cheaper than apples.</sentence>
<sentence>Enjoy your fruit!</sentence>

**Now, given the following response, please segment the response into sentences:**

**Input:**

**Response Begin**
{response}
**Response End**

**Your Sentence Segmentation Output:**`

// HallucinationValidatorPrompt is the prompt for validating sentences against
// context. Each sentence is classified as supported, unsupported,
// contradictory, disputed, or not_applicable.
const HallucinationValidatorPrompt = `You are a helpful and harmless AI assistant. You will be provided with a textual context and sentences from a model-generated response.
Your task is to analyze sentence by sentence and classify each sentence according to its relationship with the provided context.

**Instructions:**

1. **Read the textual context carefully.**
2. **For each sentence, assign one of the following labels:**
    * **` + "`supported`" + `**: The sentence is entailed by the given context. Provide a supporting excerpt from the context. The supporting except must *fully* entail the sentence.
    * **` + "`unsupported`" + `**: The sentence is not entailed by the given context. No excerpt is needed for this label.
    * **` + "`contradictory`" + `**: The sentence is falsified by the given context. Provide a contradicting excerpt from the context.
    * **` + "`disputed`" + `**: The given context contains both supporting and contradicting information. Provide both supporting and contradicting excerpt from the context.
    * **` + "`not_applicable`" + `**: The sentence does not require factual attribution (e.g., opinions, planning steps, greetings, questions, disclaimers, mathematical calculation).
3. **For each label, provide a short rationale explaining your decision.** The rationale should be separate from the excerpt.
4. **Be very strict with your ` + "`supported`" + `, ` + "`contradictory`" + ` and ` + "`disputed`" + ` decisions.** Unless you can find straightforward, indisputable evidence excepts *in the context* that a sentence is ` + "`supported`" + `, ` + "`contradictory`" + ` or ` + "`disputed`" + `, consider it ` + "`unsupported`" + `.  You should not employ world knowledge unless it is truly trivial.
5. "tool_outputs" blocks contain code execution results of the "tool_code" blocks immediately above them. If any sentence is based on "tool_outputs" results, first analyze if the corresponding "tool_code" is supported and if the results are error-free. Only if the "tool_code" block is supported, you can treat code execution results as correct.
6. If you need to cite multiple supporting excerpts, simply concatenate them. Excerpt could be summary from the context if it is too long.

**Input Format:**

The input will consist of two parts, clearly separated:

* **Context:**  The textual context used to generate the response.
* **Sentences:** The sentences from the model-generated response to be analyzed. Each sentence will be wrapped in <sentence>...</sentence>.

**Output Format:**

For each sentence, output a block of text with the following fields:

* sentence: The sentence being analyzed. Please directly copy the sentence which is provided.
* label: One of ` + "`supported`" + `, ` + "`unsupported`" + `, ` + "`contradictory`" + `, ` + "`disputed`" + ` or ` + "`not_applicable`" + `.
* rationale: A brief explanation for the assessment
* supporting_excerpt: A relevant excerpt from the context that supports the sentence. Only required for ` + "`supported`" + ` and ` + "`disputed`" + ` labels.
* contradicting_excerpt: A relevant excerpt from the context that contradicts with the sentence. Only required for ` + "`contradictory`" + ` and ` + "`disputed`" + ` labels.

**Example:**

**Input:**

**Context Begin**
Apples are red fruits. Bananas are yellow fruits. Pears are purple fruits. Pears are blue fruits.
**Context End**

**Sentences Begin**
<sentence>Apples are red.</sentence>
<sentence>Bananas are green.</sentence>
<sentence>Pears are purple.</sentence>
<sentence>Bananas are cheaper than apples.</sentence>
<sentence>Enjoy your fruit!</sentence>
**Sentences End**

**Output:**
sentence: Apples are red.
label: supported
rationale: The context explicitly states that apples are red.
supporting_excerpt: Apples are red fruits.
contradicting_excerpt: null

sentence: Bananas are green.
label: contradictory
rationale: The context states that bananas are yellow, not green.
supporting_excerpt: null
contradicting_excerpt: Bananas are yellow fruits.

sentence: Pears are purple.
label: disputed
rationale: The context states that pears are purple but it also states that pears are blue.
supporting_excerpt: Pears are purple fruits
contradicting_excerpt: Pears are blue fruits

sentence: Bananas are cheaper than apples.
label: unsupported
rationale: The context does not mention the price of bananas or apples.
supporting_excerpt: null
contradicting_excerpt: null

sentence: Enjoy your fruit!
label: not_applicable
rationale: This is a general expression and does not require factual attribution.
supporting_excerpt: null
contradicting_excerpt: null

**Now, please analyze the following context and sentences:**

**Input:**

**Context Begin**
{context}
**Context End**

**Sentences Begin**
{sentences}
**Sentences End**

**Output:**`

// PerTurnUserSimulatorQualityPrompt is the prompt template for evaluating
// user simulator quality without a persona.
const PerTurnUserSimulatorQualityPrompt = `You are an expert evaluator for a user simulator in a multi-turn conversation with an AI agent. Your task is to evaluate whether the generated user response follows the conversation plan and conversation history.

Conversation Plan:
{conversation_plan}

Conversation History:
{conversation_history}

Generated User Response:
{generated_user_response}

Stop Signal: {stop_signal}

Evaluate whether the generated user response:
1. Follows the conversation plan.
2. Is consistent with the conversation history.
3. Does not introduce information not present in the plan or history.
4. Uses the stop signal appropriately when the conversation is complete.

Answer "yes" if the response is valid, "no" otherwise.`
